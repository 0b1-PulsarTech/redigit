package gitstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

var (
	_ rewriting.History       = (*History)(nil)
	_ rewriting.ReplayHistory = (*History)(nil)
)

const (
	// step is the seconds between two rebuilt commits. A fixed step keeps a rebuild
	// byte-reproducible: two runs of the same plan produce the same identifiers, so
	// a comparison between rounds means something.
	step         = time.Second
	sha1HexLen   = 40
	sha256HexLen = 64
)

// History adapts the git writer to the rewriting stage. It supplies what the
// domain has no business knowing: who authored the rebuild, when each commit is
// dated, and the blob mode each path carries.
//
// Modes are looked up rather than assumed. A tree hashes the mode with the
// content, so rebuilding an executable script as a plain file yields a
// different tree and the invariant fails at the end without naming the cause.
type History struct {
	repoPath string
	target   string
	author   Identity
	modes    map[string]FileMode
	blobDir  string
}

func NewHistory(repoPath, target string, author Identity) *History {
	return &History{repoPath: repoPath, target: target, author: author}
}

// Write renders the domain's commits as git commits on ref.
func (h *History) Write(
	ctx context.Context,
	ref string,
	commits []rewriting.Commit,
) (string, error) {
	if err := h.loadModes(ctx); err != nil {
		return "", err
	}

	out := make([]WriteCommit, 0, len(commits))

	for index, commit := range commits {
		who := h.author
		who.When = h.author.When.Add(time.Duration(index) * step)

		writes := make([]FileWrite, 0, len(commit.Writes))
		for _, write := range commit.Writes {
			writes = append(writes, FileWrite{
				Path:    write.Path,
				Mode:    h.modes[write.Path],
				Content: write.Content,
			})
		}

		out = append(out, WriteCommit{
			Author:    who,
			Committer: who,
			Message:   commit.Subject,
			Writes:    writes,
			Deletes:   commit.Deletes,
		})
	}

	sha, err := NewWriter(h.repoPath).Write(ctx, ref, out)
	if err != nil {
		return "", fmt.Errorf("write rebuilt history: %w", err)
	}

	return sha, nil
}

// Verify compares trees, never commit identifiers: every identifier changes on
// every rebuild, so comparing them is a check that can only ever fail.
func (h *History) Verify(ctx context.Context, gotRef, wantRef string) error {
	return Check(ctx, h.repoPath, gotRef, wantRef)
}

func (h *History) WithBlobDir(dir string) *History {
	withDir := *h
	withDir.blobDir = dir

	return &withDir
}

func (h *History) Replay(
	ctx context.Context,
	ref string,
	commits []rewriting.Replayed,
) (rewriting.Staged, error) {
	out, err := h.replayCommits(commits)
	if err != nil {
		return rewriting.Staged{}, err
	}

	// The target is read before fast-import starts, so the final move can refuse a ref
	// that changed in between.
	staged, err := openStage(ctx, h.repoPath, ref)
	if err != nil {
		return rewriting.Staged{}, err
	}

	if staged.Tip, err = NewWriter(h.repoPath).Write(ctx, staged.Scratch, out); err != nil {
		cleanup := h.Discard(context.WithoutCancel(ctx), staged)

		return rewriting.Staged{}, errors.Join(
			fmt.Errorf("write replayed history: %w", err), cleanup,
		)
	}

	return staged, nil
}

func (h *History) replayCommits(commits []rewriting.Replayed) ([]WriteCommit, error) {
	out := make([]WriteCommit, 0, len(commits))

	files, err := openBlobFiles(h.blobDir)
	if err != nil {
		return nil, err
	}

	defer files.close()

	for _, commit := range commits {
		when, stampErr := stampTime(commit.At)
		if stampErr != nil {
			return nil, fmt.Errorf("stamp commit at position %d: %w", commit.Position, stampErr)
		}

		who := Identity{Name: h.author.Name, Email: h.author.Email, When: when}
		replayed := WriteCommit{Author: who, Committer: who, Message: commit.Message}

		for _, blob := range commit.Blobs {
			write, blobErr := files.write(blob)
			if blobErr != nil {
				return nil, fmt.Errorf(
					"blob %q of commit at position %d: %w", blob.Path, commit.Position, blobErr,
				)
			}

			replayed.Writes = append(replayed.Writes, write)
		}

		out = append(out, replayed)
	}

	return out, nil
}

func (h *History) loadModes(ctx context.Context) error {
	if h.modes != nil {
		return nil
	}

	modes, err := NewReader(h.repoPath, h.target).Modes(ctx)
	if err != nil {
		return fmt.Errorf("read target modes: %w", err)
	}

	h.modes = modes

	return nil
}
