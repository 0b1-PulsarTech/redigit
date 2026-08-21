package gitstore

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

var errNoCommits = errors.New("no commits to write")

// fastImportError is the fast-import process exiting non-zero. Its stderr is
// kept as a field rather than folded into Error(), for the same reason
// commandError keeps it separate: a low-cardinality message a caller can log
// without carrying whatever fast-import happened to print.
type fastImportError struct {
	Stderr string
	cause  error
}

func (e *fastImportError) Error() string { return fmt.Sprintf("run fast-import: %s", e.cause.Error()) }

func (e *fastImportError) Unwrap() error { return e.cause }

// Writer drives git fast-import as a single long-lived process, streaming an
// ordered sequence of commits into it. Measured against the alternatives at
// 300 commits x 200 files: fast-import took 4.7s against 10.9s for a
// loose-object writer plus a mandatory gc, and roughly 365s extrapolated for
// a per-blob hash-object+update-index loop.
type Writer struct {
	RepoPath string
}

func NewWriter(repoPath string) Writer { return Writer{RepoPath: repoPath} }

// Write streams commits into ref and returns the resulting commit's object
// id. The first commit is a root commit (no parent); each later commit's
// parent is the one before it in commits. --force is required to move ref
// non-fast-forward: a retry without it silently leaves ref unmoved.
func (w Writer) Write(ctx context.Context, ref string, commits []WriteCommit) (string, error) {
	if len(commits) == 0 {
		return "", fmt.Errorf("write commits: %w", errNoCommits)
	}

	// #nosec G204 -- driving the git CLI is this adapter's purpose; RepoPath
	// is caller-supplied infra config, not untrusted input.
	cmd := exec.CommandContext(ctx, "git", "-C", w.RepoPath,
		"-c", "core.ignorecase=false",
		"fast-import", "--date-format=raw", "--force")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("open fast-import stdin: %w", err)
	}

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	if err = cmd.Start(); err != nil {
		return "", fmt.Errorf("start fast-import: %w", err)
	}

	if err = streamCommits(stdin, ref, commits); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()

		return "", fmt.Errorf("stream commits: %w", err)
	}

	if err = stdin.Close(); err != nil {
		return "", fmt.Errorf("close fast-import stdin: %w", err)
	}

	if err = cmd.Wait(); err != nil {
		return "", &fastImportError{Stderr: stderr.String(), cause: err}
	}

	sha, err := revParse(ctx, w.RepoPath, ref)
	if err != nil {
		return "", fmt.Errorf("resolve written ref: %w", err)
	}

	return sha, nil
}

// streamCommits renders every commit onto dst in order, each linked to the
// one before it, then closes the stream with an explicit done command.
func streamCommits(dst io.Writer, ref string, commits []WriteCommit) error {
	w := bufio.NewWriter(dst)

	for i, commit := range commits {
		// mark i+1 identifies this commit; parentMark i is the previous
		// commit's mark, or 0 (no from line) when i is the first commit.
		if err := encodeCommit(w, ref, i+1, i, commit); err != nil {
			return fmt.Errorf("encode commit at index %d: %w", i, err)
		}
	}

	if _, err := w.WriteString("done\n"); err != nil {
		return fmt.Errorf("write done: %w", err)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush stream: %w", err)
	}

	return nil
}
