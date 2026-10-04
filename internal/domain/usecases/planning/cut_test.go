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

func TestCutNeverSplitsAnAtom(t *testing.T) {
	t.Parallel()

	// The members of one atom tie at a position and arrive with unrelated
	// declarations of other files between them; the cap of two files would cut
	// right through the gap.
	first := decl("lib/a.go", "Kind", "lib")
	first.Atom = "kind"
	second := decl("lib/z.go", "Kind.Error", "lib")
	second.Atom = "kind"

	var decls []entities.Decl

	decls = append(decls, first)

	for i := 1; i <= 4; i++ {
		decls = append(decls, decl(fmt.Sprintf("lib/m%d.go", i), fmt.Sprintf("M%d", i), "lib"))
	}

	decls = append(decls, second)

	keys := make([]entities.DeclKey, 0, len(decls))
	for _, d := range decls {
		keys = append(keys, d.Key)
	}

	commits := New(Config{MaxFiles: 2}).cut(
		entities.NewGraph(decls, nil, nil),
		entities.Ordering{},
		bucket{keys: keys, shape: entities.ShapeLayer, feature: SharedFeature, reason: "test"},
	)

	for _, commit := range commits {
		holdsFirst, holdsSecond := false, false

		for _, key := range commit.Decls {
			holdsFirst = holdsFirst || key == first.Key
			holdsSecond = holdsSecond || key == second.Key
		}

		if holdsFirst != holdsSecond {
			t.Fatalf("commit %q holds one half of an atom: %v", commit.Subject, commit.Decls)
		}
	}
}
