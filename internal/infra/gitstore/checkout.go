package gitstore

import (
	"context"
	"fmt"
	"os"
)

// Checkout is a throwaway worktree holding one revision.
//
// It exists because a language server reads the WORKING TREE, not the
// revision anyone asked about. Indexing a revision that is not checked out
// silently mismatches: positions computed from the revision's text are sent to
// a server reading different bytes, and the answers are wrong without any
// error saying so. Materialising the revision is what makes "any revision"
// true rather than "whatever happens to be checked out".
type Checkout struct {
	repoPath string
	dir      string
}

// NewCheckout materialises rev in a temporary worktree. The caller must Close
// it; a leaked worktree stays registered in the repository until pruned.
func NewCheckout(ctx context.Context, repoPath, rev string) (*Checkout, error) {
	dir, err := os.MkdirTemp("", "redigit-checkout-")
	if err != nil {
		return nil, fmt.Errorf("create checkout directory: %w", err)
	}

	// --detach because this worktree is read-only scaffolding, not a branch
	// anyone will commit on, and --force because the same revision may already
	// be checked out elsewhere in the repository.
	if _, err = runGit(
		ctx,
		repoPath,
		"worktree",
		"add",
		"--detach",
		"--force",
		dir,
		rev,
	); err != nil {
		_ = os.RemoveAll(dir)

		return nil, fmt.Errorf("check out %s: %w", rev, err)
	}

	return &Checkout{repoPath: repoPath, dir: dir}, nil
}

// Dir is the directory holding the revision.
func (c *Checkout) Dir() string { return c.dir }

// Close removes the worktree and its directory.
func (c *Checkout) Close(ctx context.Context) error {
	if _, err := runGit(ctx, c.repoPath, "worktree", "remove", "--force", c.dir); err != nil {
		// The directory is removed anyway: leaving it behind on a failed
		// unregister turns one leak into two.
		_ = os.RemoveAll(c.dir)

		return fmt.Errorf("remove checkout: %w", err)
	}

	return nil
}
