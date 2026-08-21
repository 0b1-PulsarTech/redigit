package gitstore

import (
	"context"
	"fmt"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

var _ rewriting.History = (*History)(nil)

// step is the seconds between two rebuilt commits. A fixed step keeps a rebuild
// byte-reproducible: two runs of the same plan produce the same identifiers, so
// a comparison between rounds means something.
const step = time.Second

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
