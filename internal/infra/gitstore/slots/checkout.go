package slots

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

const (
	dirMode     = 0o750
	treeDirName = "tree"
)

var (
	ErrNoFreeSlot       = errors.New("no free checkout slot")
	ErrSlotsUnsupported = errors.New("checkout slots need unix file locks")

	_ verifying.Trees = (*Trees)(nil)
)

type Trees struct {
	gitDir  string
	root    string
	mutex   sync.Mutex
	current map[int]string
	claimed map[int]string
	locks   []*os.File
}

func NewTrees(gitDir, root string) *Trees {
	return &Trees{gitDir: gitDir, root: root, current: map[int]string{}, claimed: map[int]string{}}
}

func readTreeArgs(prev, next string) []string {
	args := []string{"read-tree", "-m", "-u", "--no-sparse-checkout"}
	if prev == "" {
		return append(args, next)
	}

	return append(args, prev, next)
}

func (trees *Trees) Materialise(
	ctx context.Context,
	slot int,
	commit verifying.Commit,
) (string, error) {
	slotRoot, err := trees.claim(slot)
	if err != nil {
		return "", err
	}

	workTree, prev := filepath.Join(slotRoot, treeDirName), trees.previous(slot)
	if prev != "" && trees.advance(ctx, slotRoot, prev, commit.Tree) {
		trees.remember(slot, commit.Tree)

		return workTree, nil
	}

	trees.remember(slot, "")

	if err = os.RemoveAll(slotRoot); err != nil {
		return "", fmt.Errorf("clear slot: %w", err)
	}

	if err = os.MkdirAll(workTree, dirMode); err != nil {
		return "", fmt.Errorf("make slot dir: %w", err)
	}

	if err = trees.readTree(ctx, slotRoot, "", commit.Tree); err != nil {
		return "", err
	}

	trees.remember(slot, commit.Tree)

	return workTree, nil
}

func (trees *Trees) advance(ctx context.Context, slotRoot, prev, next string) bool {
	if !trees.isIntact(ctx, slotRoot, prev) || trees.tidy(ctx, slotRoot) != nil {
		return false
	}

	return prev == next || trees.readTree(ctx, slotRoot, prev, next) == nil
}

func (trees *Trees) claim(slot int) (string, error) {
	const maxClaims = 256

	trees.mutex.Lock()
	defer trees.mutex.Unlock()

	if known, isKnown := trees.claimed[slot]; isKnown {
		return known, nil
	}

	root, err := filepath.Abs(trees.root)
	if err != nil {
		return "", fmt.Errorf("resolve slot dir: %w", err)
	}

	if err = os.MkdirAll(root, dirMode); err != nil {
		return "", fmt.Errorf("make slots dir: %w", err)
	}

	for attempt := range maxClaims {
		name := "slot-" + strconv.Itoa(slot)
		if attempt > 0 {
			name += "-" + strconv.Itoa(attempt)
		}

		lock, isHeld, lockErr := tryLock(filepath.Join(root, name+".lock"))
		if lockErr != nil {
			return "", lockErr
		}

		if isHeld {
			trees.locks = append(trees.locks, lock)
			trees.claimed[slot] = filepath.Join(root, name)

			return trees.claimed[slot], nil
		}
	}

	return "", fmt.Errorf("%w: slot %d", ErrNoFreeSlot, slot)
}

func (trees *Trees) previous(slot int) string {
	trees.mutex.Lock()
	defer trees.mutex.Unlock()

	return trees.current[slot]
}

func (trees *Trees) remember(slot int, tree string) {
	trees.mutex.Lock()
	defer trees.mutex.Unlock()

	if tree == "" {
		delete(trees.current, slot)

		return
	}

	trees.current[slot] = tree
}

func (trees *Trees) readTree(ctx context.Context, slotRoot, prev, next string) error {
	if _, err := trees.slotGit(ctx, slotRoot, nil, readTreeArgs(prev, next)...); err != nil {
		return fmt.Errorf("check out tree: %w", err)
	}

	return nil
}

func (trees *Trees) slotGit(
	ctx context.Context,
	slotRoot string,
	stdin []byte,
	args ...string,
) ([]byte, error) {
	return runGit(ctx, gitCall{
		gitDir: trees.gitDir,
		env: []string{
			"GIT_WORK_TREE=" + filepath.Join(slotRoot, treeDirName),
			"GIT_INDEX_FILE=" + filepath.Join(slotRoot, "index"),
		},
		stdin: stdin,
		args:  args,
	})
}
