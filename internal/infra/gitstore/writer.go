package gitstore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

var errNoCommits = errors.New("no commits to write")

type fastImportError struct {
	Stderr string
	cause  error
}

func (e *fastImportError) Error() string {
	// The exit status alone says nothing about a refused stream, so stderr is folded into the text
	// here, where commandError keeps it out of Error() and in its field only.
	if e.Stderr == "" {
		return fmt.Sprintf("run fast-import: %s", e.cause.Error())
	}

	return fmt.Sprintf("run fast-import: %s: %s", e.cause.Error(), e.Stderr)
}

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

	if err := CheckRef(ref); err != nil {
		return "", fmt.Errorf("write commits: %w", err)
	}

	// Refused before the process starts, so the caller gets a typed error naming the path
	// and no process has run.
	for index, commit := range commits {
		if err := validateCommit(commit); err != nil {
			return "", fmt.Errorf("validate commit at sorted index %d: %w", index, err)
		}
	}

	runCtx, kill := context.WithCancel(ctx)
	defer kill()

	cmd, stdin, err := fastImportCmd(runCtx, w.RepoPath)
	if err != nil {
		return "", err
	}

	var stderr batch.TailBuffer

	cmd.Stderr = &stderr

	if err = cmd.Start(); err != nil {
		return "", fmt.Errorf("start fast-import: %w", err)
	}

	if err = streamCommits(stdin, ref, commits); err != nil {
		kill()

		_ = stdin.Close()

		if waitErr := cmd.Wait(); waitErr != nil {
			err = errors.Join(err, &fastImportError{Stderr: stderr.String(), cause: waitErr})
		}

		return "", fmt.Errorf("stream commits: %w", withCtxErr(ctx, err))
	}

	if err = finishImport(stdin, cmd, &stderr); err != nil {
		return "", withCtxErr(ctx, err)
	}

	sha, err := revParse(context.WithoutCancel(ctx), w.RepoPath, ref)
	if err != nil {
		return "", fmt.Errorf("resolve written ref: %w", err)
	}

	return sha, nil
}

func fastImportCmd(ctx context.Context, repoPath string) (*exec.Cmd, io.WriteCloser, error) {
	const grace = 10 * time.Second

	// #nosec G204 -- driving the git CLI is this adapter's purpose; repoPath
	// is caller-supplied infra config, not untrusted input.
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath,
		"-c", "core.ignorecase=false",
		"fast-import", "--date-format=raw", "--force", "--done")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("open fast-import stdin: %w", err)
	}

	// A kill during the final ref update strands a lock; EOF before done aborts without
	// touching refs, and a run past the grace period is killed.
	cmd.Cancel = func() error { return closeToCancel(stdin) }
	cmd.WaitDelay = grace

	return cmd, stdin, nil
}

// streamCommits renders every commit onto dst in order, each linked to the
// one before it, then closes the stream with an explicit done command.
func streamCommits(dst io.Writer, ref string, commits []WriteCommit) error {
	w := bufio.NewWriter(dst)

	for i, commit := range commits {
		// mark i+1 identifies this commit; parentMark i is the previous
		// commit's mark, or 0 (no from line) when i is the first commit.
		if err := encodeCommit(w, ref, i+1, i, commit); err != nil {
			return fmt.Errorf("encode commit at sorted index %d: %w", i, err)
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

func isOid(handle string) bool {
	isLength := len(handle) == sha1HexLen || len(handle) == sha256HexLen

	return isLength && strings.Trim(handle, "0123456789abcdef") == ""
}

func finishImport(stdin io.Closer, cmd *exec.Cmd, stderr *batch.TailBuffer) error {
	closeErr := stdin.Close()

	// The stream is fully sent: a clean exit is a success even when a late cancel reported it.
	waitErr := cmd.Wait()
	if waitErr != nil && (cmd.ProcessState == nil || !cmd.ProcessState.Success()) {
		failure := &fastImportError{Stderr: stderr.String(), cause: waitErr}
		if closeErr == nil {
			return failure
		}

		return errors.Join(fmt.Errorf("close fast-import stdin: %w", closeErr), failure)
	}

	if closeErr != nil {
		return fmt.Errorf("close fast-import stdin: %w", closeErr)
	}

	return nil
}
