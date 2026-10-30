package slots

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

func (trees *Trees) Close(context.Context) error {
	trees.mutex.Lock()
	defer trees.mutex.Unlock()

	var failures []error

	for slot, root := range trees.claimed {
		if err := os.RemoveAll(root); err != nil {
			failures = append(failures, fmt.Errorf("remove slot dir: %w", err))
		}

		delete(trees.claimed, slot)
		delete(trees.current, slot)
	}

	// Dirs go first: a lock released earlier would let another run claim a half-removed slot.
	for _, lock := range trees.locks {
		if err := lock.Close(); err != nil {
			failures = append(failures, fmt.Errorf("release slot lock: %w", err))
		}
	}

	trees.locks = nil

	return errors.Join(failures...)
}

func (trees *Trees) isIntact(ctx context.Context, slotRoot, tree string) bool {
	out, err := trees.slotGit(ctx, slotRoot, nil, "write-tree", "--missing-ok")

	return err == nil && strings.TrimSpace(string(out)) == tree
}

func (trees *Trees) tidy(ctx context.Context, slotRoot string) error {
	if _, err := trees.slotGit(ctx, slotRoot, nil, "clean", "-ffdxq"); err != nil {
		return fmt.Errorf("remove untracked files: %w", err)
	}

	changed, err := trees.slotGit(ctx, slotRoot, nil, "diff-files", "--name-only", "-z")
	if err != nil {
		return fmt.Errorf("list modified files: %w", err)
	}

	if len(bytes.TrimSpace(changed)) == 0 {
		return nil
	}

	restore := []string{"checkout-index", "-f", "-z", "--stdin"}
	if _, err = trees.slotGit(ctx, slotRoot, changed, restore...); err != nil {
		return fmt.Errorf("restore modified files: %w", err)
	}

	return nil
}
