package planning

import (
	"fmt"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
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
		bucket{keys: keys, shape: entities.ShapeLayer, feature: reach.SharedFeature, reason: "test"},
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
		bucket{keys: keys, shape: entities.ShapeLayer, feature: reach.SharedFeature, reason: "test"},
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
		bucket{keys: keys, shape: entities.ShapeLayer, feature: reach.SharedFeature, reason: "test"},
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

func TestCutKeepsADocumentApartFromCode(t *testing.T) {
	t.Parallel()

	spec := entities.DeclKey{File: "docs/specs/a.md", Kind: entities.KindOther, Name: "<file>"}
	first := decl("pkg/a.go", "A", "pkg").Key
	second := decl("pkg/b.go", "B", "pkg").Key

	commits := New(Config{}).cut(
		entities.NewGraph(nil, nil, nil),
		entities.Ordering{At: map[entities.DeclKey]entities.Position{first: 0, spec: 1, second: 2}},
		bucket{
			keys:    []entities.DeclKey{first, spec, second},
			shape:   entities.ShapeFeature,
			feature: "pkg",
			reason:  "test",
		},
	)

	if len(commits) != 3 {
		t.Fatalf("got %d commits, want code, document, code", len(commits))
	}

	if len(commits[1].Decls) != 1 || commits[1].Decls[0] != spec {
		t.Fatalf("the middle commit holds %v, want only the document", commits[1].Decls)
	}
}

func TestCycleExtentHoldsMutualAtomsTogether(t *testing.T) {
	t.Parallel()

	// Neither table's declarations need each other, but the two types do.
	first := decl("pkg/a.go", "A", "pkg")
	first.Atom = "a"
	firstTable := decl("pkg/a.sql", "TableA", "pkg")
	firstTable.Atom = "a"
	second := decl("pkg/b.go", "B", "pkg")
	second.Atom = "b"
	loose := decl("pkg/c.go", "C", "pkg")
	keys := []entities.DeclKey{first.Key, firstTable.Key, second.Key, loose.Key}
	graph := entities.NewGraph(
		[]entities.Decl{first, firstTable, second, loose},
		[]entities.Edge{needs(first, second), needs(second, firstTable)},
		nil,
	)

	got := cycleExtent(graph, keys, []int{1, 1, 2, 3})

	if want := []int{2, 2, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("extent = %v, want %v", got, want)
	}
}
