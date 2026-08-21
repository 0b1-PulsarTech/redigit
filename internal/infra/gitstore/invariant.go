package gitstore

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
)

// InvariantError is the final-tree invariant failing: the written ref's tree
// does not equal the target tree. Diverging holds the paths git diff-tree
// found different, kept structured rather than folded into Error() so a
// caller can inspect them without parsing free text.
type InvariantError struct {
	Diverging []string
}

func (e *InvariantError) Error() string {
	return fmt.Sprintf("final tree invariant failed: %d paths diverge", len(e.Diverging))
}

func (e *InvariantError) Unwrap() error { return apperr.ErrInvariant }

// Check verifies the final-tree invariant: gotRef's tree must equal wantRef's
// tree. Comparing tree object ids rather than commit shas, because a rewrite
// changes every commit sha even when the tree it produces is exactly right.
func Check(ctx context.Context, repoPath, gotRef, wantRef string) error {
	gotTree, err := treeOf(ctx, repoPath, gotRef)
	if err != nil {
		return fmt.Errorf("resolve written tree: %w", err)
	}

	wantTree, err := treeOf(ctx, repoPath, wantRef)
	if err != nil {
		return fmt.Errorf("resolve target tree: %w", err)
	}

	if gotTree == wantTree {
		return nil
	}

	diverging, err := divergingPaths(ctx, repoPath, gotTree, wantTree)
	if err != nil {
		return fmt.Errorf("list diverging paths: %w", err)
	}

	return &InvariantError{Diverging: diverging}
}

func treeOf(ctx context.Context, repoPath, ref string) (string, error) {
	return revParse(ctx, repoPath, ref+"^{tree}")
}

func divergingPaths(ctx context.Context, repoPath, treeA, treeB string) ([]string, error) {
	out, err := runGit(ctx, repoPath, "diff-tree", "-r", "-z", "--name-only", treeA, treeB)
	if err != nil {
		return nil, err
	}

	return splitNUL(out), nil
}
