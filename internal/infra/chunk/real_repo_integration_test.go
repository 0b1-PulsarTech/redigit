//go:build integration

package chunk

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// realRepoPath is the external repository the task asks this suite to prove
// itself against: 86 .sql files, 33 go.mod files, and one go.work, none of
// them fixtures this package's author could have shaped by hand. The test
// only ever reads it through "git show", never checks anything out of it.
const realRepoPath = "/var/home/jictyvoo/Documents/development/0b1pulsar_tech/terectek_comms"

const realRepoRev = "origin/main"

func TestRealRepoChunking(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat(realRepoPath); err != nil {
		t.Skipf("real repo not present at %s: %v", realRepoPath, err)
	}

	ctx := context.Background()

	files, err := gitLsTree(ctx, realRepoPath, realRepoRev)
	if err != nil {
		t.Skipf("cannot read %s at %s: %v", realRepoPath, realRepoRev, err)
	}

	queryFiles := filterPaths(files, func(path string) bool {
		return strings.Contains(path, "database/queries/") && strings.HasSuffix(path, ".sql")
	})
	goModFiles := filterPaths(
		files,
		func(path string) bool { return filepath.Base(path) == goModFileName },
	)
	goWorkFiles := filterPaths(
		files,
		func(path string) bool { return filepath.Base(path) == goWorkFileName },
	)

	t.Logf(
		"real repo: %d .sql query files, %d go.mod files, %d go.work files",
		len(queryFiles), len(goModFiles), len(goWorkFiles),
	)

	testRealSQLQueries(t, ctx, queryFiles)
	testRealGoMod(t, ctx, goModFiles)
	testRealGoWork(t, ctx, goWorkFiles)
}

// testRealGoWork is not one of the task's mandatory assertions, but the real
// go.work has something no fixture forced it to have: a "// Apps"-style
// comment line INSIDE the "use (...)" block, not just a whole line commented
// out. It must not become a phantom entry, and coverage must still hold.
func testRealGoWork(t *testing.T, ctx context.Context, goWorkFiles []string) {
	t.Helper()

	if len(goWorkFiles) == 0 {
		t.Fatal("expected a go.work in the real repo, found none")
	}

	for _, path := range goWorkFiles {
		source, err := gitShow(ctx, realRepoPath, realRepoRev, path)
		if err != nil {
			t.Fatalf("git show %s: %v", path, err)
		}

		decls, err := (GoWork{}).Chunks(ctx, path, source)
		if err != nil {
			t.Fatalf("Chunks(%s): %v", path, err)
		}

		assertReconstructs(t, string(source), decls)
		t.Logf("real repo: %s yields %d chunks", path, len(decls))
	}
}

func testRealSQLQueries(t *testing.T, ctx context.Context, queryFiles []string) {
	t.Helper()

	if len(queryFiles) == 0 {
		t.Fatal("expected database/queries/*.sql files in the real repo, found none")
	}

	totalQueries := 0

	for _, path := range queryFiles {
		source, err := gitShow(ctx, realRepoPath, realRepoRev, path)
		if err != nil {
			t.Fatalf("git show %s: %v", path, err)
		}

		decls, err := (SQLQueries{}).Chunks(ctx, path, source)
		if err != nil {
			t.Fatalf("Chunks(%s): %v", path, err)
		}

		if len(decls) == 0 {
			t.Fatalf("%s: got 0 chunks, want at least 1", path)
		}

		assertReconstructs(t, string(source), decls)

		totalQueries += len(decls)
	}

	t.Logf("real repo: %d total chunks across %d query files", totalQueries, len(queryFiles))
}

func testRealGoMod(t *testing.T, ctx context.Context, goModFiles []string) {
	t.Helper()

	if len(goModFiles) == 0 {
		t.Fatal("expected go.mod files in the real repo, found none")
	}

	totalLines := 0

	for _, path := range goModFiles {
		source, err := gitShow(ctx, realRepoPath, realRepoRev, path)
		if err != nil {
			t.Fatalf("git show %s: %v", path, err)
		}

		decls, err := (GoMod{}).Chunks(ctx, path, source)
		if err != nil {
			t.Fatalf("Chunks(%s): %v", path, err)
		}

		lines := splitLines(source)
		wantEntries := len(scanDirective(source, lines, "require")) +
			len(scanDirective(source, lines, "replace"))

		if len(decls) != wantEntries+1 {
			t.Fatalf(
				"%s: got %d decls, want %d (1 header + %d require/replace lines)",
				path, len(decls), wantEntries+1, wantEntries,
			)
		}

		totalLines += wantEntries
	}

	t.Logf(
		"real repo: %d go.mod files, %d require/replace lines total",
		len(goModFiles), totalLines,
	)
}

func filterPaths(paths []string, keep func(string) bool) []string {
	var out []string

	for _, path := range paths {
		if keep(path) {
			out = append(out, path)
		}
	}

	return out
}

func gitLsTree(ctx context.Context, repo, rev string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "ls-tree", "-r", rev, "--name-only").
		Output()
	if err != nil {
		return nil, err //nolint:wrapcheck // exec errors carry their own actionable stderr
	}

	return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), nil
}

func gitShow(ctx context.Context, repo, rev, path string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "show", rev+":"+path).Output()
	if err != nil {
		return nil, err //nolint:wrapcheck // exec errors carry their own actionable stderr
	}

	return out, nil
}
