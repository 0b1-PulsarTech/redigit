package planning

import (
	"fmt"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func packageOf(commit entities.PlannedCommit) map[string]bool {
	dirs := map[string]bool{}
	for _, key := range commit.Decls {
		dirs[dirOf(key.File)] = true
	}

	return dirs
}

func TestCutPrefersAPackageBoundaryOverTheRawCap(t *testing.T) {
	t.Parallel()

	var keys []entities.DeclKey

	for i := 1; i <= 8; i++ {
		keys = append(keys, decl(fmt.Sprintf("pkga/f%d.go", i), fmt.Sprintf("F%d", i), "pkga").Key)
	}

	for i := 1; i <= 6; i++ {
		keys = append(keys, decl(fmt.Sprintf("pkgb/f%d.go", i), fmt.Sprintf("F%d", i), "pkgb").Key)
	}

	commits := New(Config{MaxFiles: 12}).cut(
		entities.NewGraph(nil, nil, nil),
		entities.Ordering{},
		bucket{keys: keys, shape: entities.ShapeLayer, feature: SharedFeature, reason: "test"},
	)

	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2: one per package", len(commits))
	}

	for _, commit := range commits {
		if dirs := packageOf(commit); len(dirs) != 1 {
			t.Fatalf("commit %q spans packages %v, want exactly one", commit.Subject, dirs)
		}
	}

	if len(commits[0].Decls) != 8 || len(commits[1].Decls) != 6 {
		t.Fatalf(
			"commit sizes = %d, %d; want 8 (pkga) then 6 (pkgb), not split at the raw file cap",
			len(commits[0].Decls), len(commits[1].Decls),
		)
	}
}

func TestCutFallsBackToTheCapWhenOnePackageAloneExceedsIt(t *testing.T) {
	t.Parallel()

	var keys []entities.DeclKey
	for i := 1; i <= 9; i++ {
		keys = append(keys, decl(fmt.Sprintf("lib/f%d.go", i), fmt.Sprintf("F%d", i), "lib").Key)
	}

	commits := New(Config{MaxFiles: 3}).cut(
		entities.NewGraph(nil, nil, nil),
		entities.Ordering{},
		bucket{keys: keys, shape: entities.ShapeLayer, feature: SharedFeature, reason: "test"},
	)

	if len(commits) != 3 {
		t.Fatalf("got %d commits, want 3: one lib package split by the cap alone", len(commits))
	}

	for _, commit := range commits {
		if len(commit.Decls) > 3 {
			t.Fatalf(
				"commit %q touches %d files, want at most 3",
				commit.Subject,
				len(commit.Decls),
			)
		}
	}
}
