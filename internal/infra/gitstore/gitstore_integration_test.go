//go:build integration

package gitstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newTestRepo initializes a fresh, throwaway git repository under t.TempDir()
// and returns its path. Every integration test in this package builds its
// own repo rather than sharing one across tests.
func newTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runTestGit(t, dir, "init", "-q", "-b", "main")
	runTestGit(t, dir, "config", "user.email", "tester@example.com")
	runTestGit(t, dir, "config", "user.name", "Tester")

	return dir
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// commitTestFiles writes every path in files under dir and commits them all,
// seeding a source-history fixture the reader/writer tests read back.
func commitTestFiles(t *testing.T, dir, message string, files map[string][]byte) {
	t.Helper()

	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}

		if err := os.WriteFile(full, content, 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	runTestGit(t, dir, "add", "-A")
	runTestGit(t, dir, "commit", "-q", "-m", message)
}
