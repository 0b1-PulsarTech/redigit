package golang

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestRulePackName(t *testing.T) {
	t.Parallel()

	if got := NewRulePack(nil).Name(); got != "go" {
		t.Fatalf("Name() = %q, want %q", got, "go")
	}
}

func TestRulePackEdgesUnionsEveryRule(t *testing.T) {
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

	pack := NewRulePack(nil)

	edges, err := pack.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	found := false

	for _, edge := range edges {
		if edge.Reason == entities.ReasonEncloses {
			found = true
		}
	}

	if !found {
		t.Fatalf("Edges() did not include the encloses rule's result: %+v", edges)
	}
}

func TestRulePackAnchorsUnionsEveryRule(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, "fs.go", `package sample

type FS struct{}

func (FS) Get(key string) ([]byte, error) { return nil, nil }
`)
	writeFile(t, root, "fs_test.go", `package sample

import "testing"

func TestFSGet(t *testing.T) {}
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{"fs.go", "fs_test.go"})

	anchors, err := NewRulePack(nil).Anchors(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	if len(anchors) != 1 {
		t.Fatalf("got %d anchors, want 1: %+v", len(anchors), anchors)
	}
}

// stubFailingRule proves Edges/Anchors report a failing rule's name in the
// wrapped error, rather than swallowing which rule broke.
type stubFailingRule struct{ name string }

func (r stubFailingRule) Name() string { return r.name }

func (stubFailingRule) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, errors.New("boom")
}

func (stubFailingRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, errors.New("boom")
}

func TestRulePackWrapsFailingRuleName(t *testing.T) {
	t.Parallel()

	stub := []rule{stubFailingRule{name: "stub"}}

	_, err := collectEdges(context.Background(), entities.Snapshot{}, nil, stub)
	if err == nil || !strings.Contains(err.Error(), "stub") {
		t.Fatalf("collectEdges() error = %v, want it to name the failing rule", err)
	}

	_, err = collectAnchors(context.Background(), entities.Snapshot{}, nil, stub)
	if err == nil || !strings.Contains(err.Error(), "stub") {
		t.Fatalf("collectAnchors() error = %v, want it to name the failing rule", err)
	}
}
