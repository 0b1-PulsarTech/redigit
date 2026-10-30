//go:build integration

package slots

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

type fixtureFile struct {
	path    string
	content string
	mode    os.FileMode
	link    string
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")

	return dir
}

func commitFiles(t *testing.T, dir, message string, files []fixtureFile) verifying.Commit {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	for _, entry := range entries {
		if entry.Name() != ".git" {
			if err = os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
				t.Fatalf("clear: %v", err)
			}
		}
	}

	for _, file := range files {
		full := filepath.Join(dir, filepath.FromSlash(file.path))
		if err = os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if file.link != "" {
			err = os.Symlink(file.link, full)
		} else {
			err = os.WriteFile(full, []byte(file.content), file.mode)
		}

		if err != nil {
			t.Fatalf("write %s: %v", file.path, err)
		}
	}

	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", message)

	return verifying.Commit{
		Oid:     gitIn(t, dir, "rev-parse", "HEAD"),
		Tree:    gitIn(t, dir, "rev-parse", "HEAD^{tree}"),
		Subject: message,
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()

	got := map[string]string{}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, _ := filepath.Rel(root, path)
		info, infoErr := entry.Info()

		switch {
		case infoErr != nil:
			return fmt.Errorf("stat %s: %w", path, infoErr)
		case entry.IsDir():
			got[rel] = "dir"
		case info.Mode()&os.ModeSymlink != 0:
			target, _ := os.Readlink(path)
			got[rel] = "link:" + target
		default:
			content, _ := os.ReadFile(path)
			got[rel] = string(content) + "|" + info.Mode().Perm().String()
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	return got
}

func TestTreesIncrementalCheckout(t *testing.T) {
	repo := newRepo(t)
	gitDir := filepath.Join(repo, ".git")
	indexBefore, _ := os.ReadFile(filepath.Join(gitDir, "index"))

	first := commitFiles(t, repo, "first", []fixtureFile{
		{path: "a.txt", content: "one", mode: 0o644},
		{path: "dir/b.sh", content: "#!/bin/sh", mode: 0o644},
		{path: "gone/c.txt", content: "bye", mode: 0o644},
		{path: "ln", link: "a.txt"},
	})
	second := commitFiles(t, repo, "second", []fixtureFile{
		{path: "a.txt", content: "one", mode: 0o644},
		{path: "dir/b.sh", content: "#!/bin/sh", mode: 0o755},
		{path: "new/d.txt", content: "added", mode: 0o644},
		{path: "ln", link: "new/d.txt"},
	})
	indexAfterCommits, _ := os.ReadFile(filepath.Join(gitDir, "index"))

	if string(indexBefore) == string(indexAfterCommits) {
		t.Fatal("fixture: the repo index never changed")
	}

	trees := NewTrees(gitDir, t.TempDir())
	ctx := t.Context()

	dir, err := trees.Materialise(ctx, 0, first)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	want := map[string]string{
		".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir", "dir/b.sh": "#!/bin/sh|-rw-r--r--",
		"gone": "dir", "gone/c.txt": "bye|-rw-r--r--", "ln": "link:a.txt",
	}
	checkSnapshot(t, dir, want)

	again, err := trees.Materialise(ctx, 0, second)
	if err != nil || again != dir {
		t.Fatalf("second: dir = %q err = %v, want the same slot dir %q", again, err, dir)
	}

	checkSnapshot(t, dir, map[string]string{
		".": "dir", "a.txt": "one|-rw-r--r--", "dir": "dir", "dir/b.sh": "#!/bin/sh|-rwxr-xr-x",
		"new": "dir", "new/d.txt": "added|-rw-r--r--", "ln": "link:new/d.txt",
	})

	indexAfterSlots, _ := os.ReadFile(filepath.Join(gitDir, "index"))
	if string(indexAfterSlots) != string(indexAfterCommits) {
		t.Fatal("the repository index changed: the slot must use a private index")
	}
}

func checkSnapshot(t *testing.T, dir string, want map[string]string) {
	t.Helper()

	got := snapshot(t, dir)

	for path, content := range want {
		if got[path] != content {
			t.Errorf("%s = %q, want %q", path, got[path], content)
		}
	}

	for path := range got {
		if _, isWanted := want[path]; !isWanted {
			t.Errorf("unexpected leftover %s", path)
		}
	}
}

func TestTreesCloseRemovesSlotDirsAndReleasesLocks(t *testing.T) {
	repo := newRepo(t)
	commit := commitFiles(
		t,
		repo,
		"only",
		[]fixtureFile{{path: "f.txt", content: "1", mode: 0o644}},
	)
	root, gitDir := t.TempDir(), filepath.Join(repo, ".git")
	first, second := NewTrees(gitDir, root), NewTrees(gitDir, root)

	for _, trees := range []*Trees{first, second} {
		if _, err := trees.Materialise(t.Context(), 0, commit); err != nil {
			t.Fatalf("materialise: %v", err)
		}
	}

	for _, name := range []string{"slot-0", "slot-0-1"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s should exist while claimed: %v", name, err)
		}
	}

	for _, trees := range []*Trees{first, second} {
		if err := trees.Close(t.Context()); err != nil {
			t.Fatalf("close: %v", err)
		}
	}

	for _, name := range []string{"slot-0", "slot-0-1"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survived Close: %v", name, err)
		}

		if _, err := os.Stat(filepath.Join(root, name+".lock")); err != nil {
			t.Fatalf("%s.lock should stay: %v", name, err)
		}
	}

	again := NewTrees(gitDir, root)
	if _, err := again.Materialise(t.Context(), 0, commit); err != nil {
		t.Fatalf("a released slot must be claimable again: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "slot-0")); err != nil {
		t.Fatalf("the reclaimed slot should be slot-0: %v", err)
	}
}

func TestTreesIndependentSlots(t *testing.T) {
	repo := newRepo(t)
	first := commitFiles(
		t,
		repo,
		"first",
		[]fixtureFile{{path: "f.txt", content: "1", mode: 0o644}},
	)
	second := commitFiles(
		t,
		repo,
		"second",
		[]fixtureFile{{path: "f.txt", content: "2", mode: 0o644}},
	)
	trees := NewTrees(filepath.Join(repo, ".git"), t.TempDir())

	left, err := trees.Materialise(t.Context(), 0, first)
	if err != nil {
		t.Fatalf("slot 0: %v", err)
	}

	right, err := trees.Materialise(t.Context(), 1, second)
	if err != nil {
		t.Fatalf("slot 1: %v", err)
	}

	if left == right {
		t.Fatal("two slots share a directory")
	}

	checkSnapshot(t, left, map[string]string{".": "dir", "f.txt": "1|-rw-r--r--"})
	checkSnapshot(t, right, map[string]string{".": "dir", "f.txt": "2|-rw-r--r--"})
}

func TestTreesRecoversFromAnUntrackedClash(t *testing.T) {
	repo := newRepo(t)
	first := commitFiles(
		t,
		repo,
		"first",
		[]fixtureFile{{path: "a.txt", content: "1", mode: 0o644}},
	)
	second := commitFiles(t, repo, "second", []fixtureFile{
		{path: "a.txt", content: "1", mode: 0o644},
		{path: "built.out", content: "tracked now", mode: 0o644},
	})
	trees := NewTrees(filepath.Join(repo, ".git"), t.TempDir())

	dir, err := trees.Materialise(t.Context(), 0, first)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	if err = os.WriteFile(
		filepath.Join(dir, "built.out"),
		[]byte("a build product"),
		0o600,
	); err != nil {
		t.Fatalf("product: %v", err)
	}

	if _, err = trees.Materialise(t.Context(), 0, second); err != nil {
		t.Fatalf("second: %v", err)
	}

	checkSnapshot(t, dir, map[string]string{
		".": "dir", "a.txt": "1|-rw-r--r--", "built.out": "tracked now|-rw-r--r--",
	})
}

func TestTreesFailures(t *testing.T) {
	repo := newRepo(t)
	first := commitFiles(t, repo, "first", []fixtureFile{{path: "a", content: "1", mode: 0o644}})

	testCases := []struct {
		name   string
		trees  *Trees
		commit verifying.Commit
	}{
		{
			"an unknown tree",
			NewTrees(filepath.Join(repo, ".git"), t.TempDir()),
			verifying.Commit{Tree: strings.Repeat("0", 40)},
		},
		{"no repository", NewTrees(filepath.Join(repo, "nothing"), t.TempDir()), first},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := testCase.trees.Materialise(t.Context(), 0, testCase.commit); err == nil {
				t.Fatal("want an error, never a directory to build in")
			}
		})
	}
}

func TestCommitsAndModules(t *testing.T) {
	repo := newRepo(t)
	commitFiles(t, repo, "docs only", []fixtureFile{{path: "README", content: "x", mode: 0o644}})
	commitFiles(t, repo, "[wip] modules", []fixtureFile{
		{
			path:    "go.mod",
			content: "module ex/root\n\ngo 1.26\n\nrequire ex/lib v0.0.0\n",
			mode:    0o644,
		},
		{path: "lib/go.mod", content: "module ex/lib\n\ngo 1.25\n", mode: 0o644},
		{path: "app/pubspec.yaml", content: "name: app\n", mode: 0o644},
	})

	gitDir := filepath.Join(repo, ".git")

	commits, err := NewCommits(gitDir).List(t.Context(), "main")
	if err != nil || len(commits) != 2 || commits[1].Subject != "[wip] modules" ||
		commits[1].Position != 2 {
		t.Fatalf("commits = %+v err = %v", commits, err)
	}

	if _, err = NewCommits(gitDir).List(t.Context(), "nope"); err == nil {
		t.Fatal("an unknown ref must fail")
	}

	lister := NewModules(gitDir, map[string]string{"go.mod": "go", "pubspec.yaml": "dart"})

	none, err := lister.List(t.Context(), commits[0])
	if err != nil || len(none.Modules) != 0 || len(none.Strays) != 0 {
		t.Fatalf("docs-only listing = %+v err = %v", none, err)
	}

	got, err := lister.List(t.Context(), commits[1])
	if err != nil || len(got.Modules) != 3 || len(got.Strays) != 0 {
		t.Fatalf("listing = %+v err = %v", got, err)
	}

	checkModules(t, repo, got.Modules, commits[1].Tree)
}

func checkModules(t *testing.T, repo string, got []verifying.Module, tree string) {
	t.Helper()

	byDir := map[string]verifying.Module{}
	for _, module := range got {
		byDir[module.Dir] = module
	}

	root, lib, app := byDir["."], byDir["lib"], byDir["app"]
	if root.Path != "ex/root" || root.GoVersion != "1.26" || root.Subtree != tree ||
		len(root.Requires) != 1 || root.Requires[0] != "ex/lib" {
		t.Fatalf("root = %+v", root)
	}

	if lib.Path != "ex/lib" || lib.Language != "go" ||
		lib.Subtree != gitIn(t, repo, "rev-parse", "HEAD:lib") {
		t.Fatalf("lib = %+v", lib)
	}

	if app.Language != "dart" || app.Path != "app" ||
		app.Subtree != gitIn(t, repo, "rev-parse", "HEAD:app") {
		t.Fatalf("app = %+v", app)
	}
}
