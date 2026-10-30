//go:build integration

package slots

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func twoTrees(t *testing.T) (repo string, first, second verifying.Commit) {
	t.Helper()

	repo = newRepo(t)
	first = commitFiles(t, repo, "first", []fixtureFile{
		{path: "a.txt", content: "one", mode: 0o644},
		{path: "dir/b.txt", content: "two", mode: 0o644},
	})
	second = commitFiles(t, repo, "second", []fixtureFile{
		{path: "a.txt", content: "one", mode: 0o644},
		{path: "dir/b.txt", content: "three", mode: 0o644},
		{path: "c.txt", content: "four", mode: 0o644},
	})

	return repo, first, second
}

func writeAt(t *testing.T, dir, name, content string) {
	t.Helper()

	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestTreesTidiesWhatTheCheckLeftBehind(t *testing.T) {
	repo, first, second := twoTrees(t)
	wantFirst := map[string]string{
		".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir", "dir/b.txt": "two|-rw-r--r--",
	}

	testCases := []struct {
		name   string
		damage func(t *testing.T, dir string)
		next   verifying.Commit
		want   map[string]string
	}{
		{
			"an untracked file and directory are removed on the way to another tree",
			func(t *testing.T, dir string) {
				writeAt(t, dir, "junk.o", "x")
				writeAt(t, dir, "out/deep/y.bin", "y")
			},
			second,
			map[string]string{
				".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir",
				"dir/b.txt": "three|-rw-r--r--", "c.txt": "four|-rw-r--r--",
			},
		},
		{
			"an untracked file is removed when the tree is unchanged",
			func(t *testing.T, dir string) { writeAt(t, dir, "junk.o", "x") },
			first, wantFirst,
		},
		{
			"a modified tracked file is restored when the tree is unchanged",
			func(t *testing.T, dir string) { writeAt(t, dir, "a.txt", "tampered") },
			first, wantFirst,
		},
		{
			"a deleted tracked file is restored when the tree is unchanged",
			func(t *testing.T, dir string) {
				if err := os.Remove(filepath.Join(dir, "dir", "b.txt")); err != nil {
					t.Fatalf("remove: %v", err)
				}
			},
			first, wantFirst,
		},
		{
			"a modified tracked file does not survive the move to another tree",
			func(t *testing.T, dir string) { writeAt(t, dir, "dir/b.txt", "tampered") },
			second,
			map[string]string{
				".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir",
				"dir/b.txt": "three|-rw-r--r--", "c.txt": "four|-rw-r--r--",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			trees := NewTrees(filepath.Join(repo, ".git"), t.TempDir())

			dir, err := trees.Materialise(t.Context(), 0, first)
			if err != nil {
				t.Fatalf("first: %v", err)
			}

			testCase.damage(t, dir)

			if _, err = trees.Materialise(t.Context(), 0, testCase.next); err != nil {
				t.Fatalf("next: %v", err)
			}

			checkSnapshot(t, dir, testCase.want)
		})
	}
}

func TestTreesIgnoreRepositoryCheckoutConfig(t *testing.T) {
	repo, _, second := twoTrees(t)
	gitIn(t, repo, "config", "core.sparseCheckout", "true")
	writeAt(t, filepath.Join(repo, ".git", "info"), "sparse-checkout", "/a.txt\n")

	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	marker := filepath.Join(t.TempDir(), "ran")
	writeAt(t, filepath.Dir(hook), "fsmonitor.sh", "#!/bin/sh\ntouch "+marker+"\n")

	if err := os.Chmod(hook, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	gitIn(t, repo, "config", "core.fsmonitor", hook)

	trees := NewTrees(filepath.Join(repo, ".git"), t.TempDir())

	dir, err := trees.Materialise(t.Context(), 0, second)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}

	checkSnapshot(t, dir, map[string]string{
		".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir",
		"dir/b.txt": "three|-rw-r--r--", "c.txt": "four|-rw-r--r--",
	})

	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("the repository's fsmonitor hook ran inside a slot")
	}
}

func TestTreesIgnoreRepositoryStatConfig(t *testing.T) {
	repo, first, _ := twoTrees(t)
	gitIn(t, repo, "config", "core.ignoreStat", "true")
	gitIn(t, repo, "config", "core.checkStat", "minimal")

	trees := NewTrees(filepath.Join(repo, ".git"), t.TempDir())

	dir, err := trees.Materialise(t.Context(), 0, first)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	writeAt(t, dir, "a.txt", "tampered")

	if _, err = trees.Materialise(t.Context(), 0, first); err != nil {
		t.Fatalf("again: %v", err)
	}

	checkSnapshot(t, dir, map[string]string{
		".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir", "dir/b.txt": "two|-rw-r--r--",
	})
}

func TestTreesRebuildWhenTheIndexIsNotThePreviousTree(t *testing.T) {
	repo, first, second := twoTrees(t)
	other := commitFiles(t, repo, "other", []fixtureFile{
		{path: "z.txt", content: "elsewhere", mode: 0o644},
	})
	trees := NewTrees(filepath.Join(repo, ".git"), t.TempDir())

	dir, err := trees.Materialise(t.Context(), 0, first)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	cmd := exec.CommandContext(
		t.Context(), "git", "--git-dir", filepath.Join(repo, ".git"), "--work-tree", dir,
		"read-tree", "--reset", "-u", other.Tree,
	)

	cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+filepath.Join(filepath.Dir(dir), "index"))

	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		t.Fatalf("tamper: %v\n%s", runErr, out)
	}

	if _, err = trees.Materialise(t.Context(), 0, second); err != nil {
		t.Fatalf("second: %v", err)
	}

	checkSnapshot(t, dir, map[string]string{
		".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir",
		"dir/b.txt": "three|-rw-r--r--", "c.txt": "four|-rw-r--r--",
	})
}

func TestTreesShareNoSlotAcrossProcesses(t *testing.T) {
	repo, first, second := twoTrees(t)
	root := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	left, right := NewTrees(gitDir, root), NewTrees(gitDir, root)

	leftDir, err := left.Materialise(t.Context(), 0, first)
	if err != nil {
		t.Fatalf("left: %v", err)
	}

	rightDir, err := right.Materialise(t.Context(), 0, second)
	if err != nil {
		t.Fatalf("right: %v", err)
	}

	if leftDir == rightDir {
		t.Fatalf("two users of one cache dir share the slot %s", leftDir)
	}

	checkSnapshot(t, leftDir, map[string]string{
		".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir", "dir/b.txt": "two|-rw-r--r--",
	})
	checkSnapshot(t, rightDir, map[string]string{
		".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir",
		"dir/b.txt": "three|-rw-r--r--", "c.txt": "four|-rw-r--r--",
	})
}
