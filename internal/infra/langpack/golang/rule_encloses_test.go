package golang

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestEnclosesRuleValueTypedWithSameFileType(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, "weekday.go", `package sample

type Weekday int

const (
	Sunday Weekday = iota
	Monday
)
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{"weekday.go"})

	edges, err := enclosesRule{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	want := entities.Edge{
		Dependent: entities.DeclKey{File: "weekday.go", Kind: entities.KindValue, Name: "Sunday"},
		Provider:  entities.DeclKey{File: "weekday.go", Kind: entities.KindType, Name: "Weekday"},
		Reason:    entities.ReasonEncloses,
	}

	if len(edges) != 1 || edges[0] != want {
		t.Fatalf("edges = %+v, want [%+v]", edges, want)
	}
}

func TestEnclosesRuleConstructorWithSoleResultType(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, "repo.go", `package sample

type Repo struct{}

func New() *Repo { return &Repo{} }

func NewWithErr() (*Repo, error) { return nil, nil }
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{"repo.go"})

	edges, err := enclosesRule{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	want := entities.Edge{
		Dependent: entities.DeclKey{File: "repo.go", Kind: entities.KindFunc, Name: "New"},
		Provider:  entities.DeclKey{File: "repo.go", Kind: entities.KindType, Name: "Repo"},
		Reason:    entities.ReasonEncloses,
	}

	if len(edges) != 1 || edges[0] != want {
		t.Fatalf(
			"edges = %+v, want [%+v] (NewWithErr has two results, so it must not match)",
			edges,
			want,
		)
	}
}

func TestEnclosesRuleUnrelatedTypeNoEdge(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, "misc.go", `package sample

type Repo struct{}

const Timeout = 5

func Helper() int { return 1 }
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{"misc.go"})

	edges, err := enclosesRule{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edges for declarations untyped by Repo, want 0: %+v", len(edges), edges)
	}
}

func TestEnclosesRuleBlankIdentifierSkipped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, "assert.go", `package sample

import "io"

type Reader struct{}

var _ io.Reader = (*Reader)(nil)
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{"assert.go"})

	edges, err := enclosesRule{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf(
			"got %d edges for a blank-identifier assertion typed by an unrelated interface, want 0: %+v",
			len(edges),
			edges,
		)
	}
}

func TestEnclosesRuleAnchorsIsEmpty(t *testing.T) {
	t.Parallel()

	anchors, err := enclosesRule{}.Anchors(context.Background(), entities.Snapshot{}, nil)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	if len(anchors) != 0 {
		t.Fatalf("got %d anchors, want 0 (encloses is an Edge rule)", len(anchors))
	}
}

func TestEnclosesRuleNoTypesInFileSkipsRead(t *testing.T) {
	t.Parallel()

	// Root has no files on disk; if the rule tried to read misc.go here it
	// would fail. A file with no type declarations at all must never reach
	// readSource.
	snap := entities.Snapshot{Root: t.TempDir()}
	decls := []entities.Decl{
		{Key: entities.DeclKey{File: "misc.go", Kind: entities.KindFunc, Name: "Helper"}},
	}

	edges, err := enclosesRule{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edges, want 0", len(edges))
	}
}
