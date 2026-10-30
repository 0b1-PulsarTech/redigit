//go:build integration

package slots

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitDir(t *testing.T) {
	repo := newRepo(t)
	bare := filepath.Join(t.TempDir(), "bare.git")
	gitIn(t, repo, "init", "-q", "--bare", bare)

	testCases := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{"a work tree resolves to its .git", repo, mustEval(t, filepath.Join(repo, ".git")), false},
		{"a bare repository is its own git dir", bare, mustEval(t, bare), false},
		{"not a repository", t.TempDir(), "", true},
		{"a missing path", filepath.Join(repo, "missing"), "", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := GitDir(t.Context(), testCase.path)
			if (err != nil) != testCase.wantErr ||
				(err == nil && mustEval(t, got) != testCase.want) {
				t.Fatalf(
					"dir = %q err = %v, want %q (error %v)",
					got,
					err,
					testCase.want,
					testCase.wantErr,
				)
			}
		})
	}
}

func TestGitDirAndTreesIgnoreAnInheritedGitEnvironment(t *testing.T) {
	repo := newRepo(t)
	other := newRepo(t)
	commit := commitFiles(t, repo, "mine", []fixtureFile{
		{path: "a.txt", content: "one", mode: 0o644},
	})

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "index"))
	t.Setenv("GIT_WORK_TREE", other)

	got, err := GitDir(t.Context(), repo)
	if err != nil || mustEval(t, got) != mustEval(t, filepath.Join(repo, ".git")) {
		t.Fatalf("GitDir = %q err = %v, want the --repo's own git dir", got, err)
	}

	dir, err := NewTrees(got, t.TempDir()).Materialise(t.Context(), 0, commit)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}

	checkSnapshot(t, dir, map[string]string{".": "dir", "a.txt": "one|-rw-r--r--"})

	if entries, readErr := os.ReadDir(other); readErr != nil || len(entries) != 1 {
		t.Fatalf("the inherited work tree was touched: %v err = %v", entries, readErr)
	}
}

func mustEval(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}

	return resolved
}
