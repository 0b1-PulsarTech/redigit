//go:build integration

package bootstrap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStart runs the real Start against a throwaway git repository holding
// one tiny Go module: the one path in this package that names a concrete
// adapter, and the one this task's unit tests cannot reach without spawning a
// language server and creating a git worktree.
func TestStart(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not on PATH")
	}

	dir := t.TempDir()
	writeFixtureModule(t, dir)
	initFixtureRepo(t, dir)

	pipeline, err := Start(t.Context(), Config{
		Repo:          dir,
		Rev:           "HEAD",
		ServerCommand: "gopls",
		ServerArgs:    []string{"-mode=stdio"},
	})
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}

	result, err := pipeline.Graph(t.Context())
	if err != nil {
		t.Fatalf("Graph() = %v", err)
	}

	if result.Graph.Len() == 0 {
		t.Fatal("expected at least one declaration in the graph")
	}

	// Close's own error-folding is pinned deterministically in
	// pipeline_test.go; a real gopls process can exit on its own the instant
	// shutdown/exit lands, racing the wait that follows and reporting a
	// broken pipe that carries no information about the checkout this
	// assertion cares about. What must hold here is the real-world
	// consequence: the checkout is gone, session error or not.
	if err := pipeline.Close(context.WithoutCancel(t.Context())); err != nil {
		t.Logf("Close() = %v (non-fatal: only the checkout cleanup is asserted below)", err)
	}

	if got := fixtureWorktreeCount(t, dir); got != 1 {
		t.Fatalf("worktree count after Close = %d, want 1 (the repo's own)", got)
	}
}

// writeFixtureModule seeds a repository with one Go file holding a
// declaration and a caller, so the index has at least one edge to find.
func writeFixtureModule(t testing.TB, dir string) {
	t.Helper()

	files := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.26\n",
		"main.go": "package fixture\n\n" +
			"func Target() int { return 1 }\n\n" +
			"func Caller() int { return Target() }\n",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func initFixtureRepo(t testing.TB, dir string) {
	t.Helper()

	runFixtureGit(t, dir, "init", "-q", "-b", "main")
	runFixtureGit(t, dir, "config", "user.email", "tester@example.com")
	runFixtureGit(t, dir, "config", "user.name", "Tester")
	runFixtureGit(t, dir, "add", "-A")
	runFixtureGit(t, dir, "commit", "-q", "-m", "fixture module")
}

func runFixtureGit(t testing.TB, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// fixtureWorktreeCount counts the worktrees git still has registered for
// dir's repository. Close is supposed to leave only the repository's own.
func fixtureWorktreeCount(t testing.TB, dir string) int {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", "-C", dir, "worktree", "list", "--porcelain").
		CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}

	count := 0

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			count++
		}
	}

	return count
}
