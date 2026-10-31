package golang

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
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

	testkit.WriteFile(t, root, "weekday.go", `package sample

type Weekday int

const (
	Sunday Weekday = iota
	Monday
)
`)

	snap := entities.Snapshot{Root: root}

	decls := testkit.DeclareAll(t, root, []string{"weekday.go"})

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

	testkit.WriteFile(t, root, "fs.go", `package sample

type FS struct{}

func (FS) Get(key string) ([]byte, error) { return nil, nil }
`)
	testkit.WriteFile(t, root, "fs_test.go", `package sample

import "testing"

func TestFSGet(t *testing.T) {}
`)

	snap := entities.Snapshot{Root: root}

	decls := testkit.DeclareAll(t, root, []string{"fs.go", "fs_test.go"})

	anchors, err := NewRulePack(nil).Anchors(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	if len(anchors) != 1 {
		t.Fatalf("got %d anchors, want 1: %+v", len(anchors), anchors)
	}
}

type stubFailingRule struct{ name string }

func (stub stubFailingRule) Name() string { return stub.name }

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

func TestRulePackLeavesOtherLanguagesAlone(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	testkit.WriteFile(t, root, "lib/money.dart", "import 'x.dart';\n\nfinal class Money {}\n")
	testkit.WriteFile(t, root, "test/money_test.dart", "void main() {}\n")

	snap := entities.Snapshot{Root: root, Files: []string{"lib/money.dart", "test/money_test.dart"}}
	decls := []entities.Decl{
		{
			Key:  entities.DeclKey{File: "lib/money.dart", Kind: entities.KindType, Name: "Money"},
			Span: entities.Span{Start: 18, End: 38},
		},
		{
			Key: entities.DeclKey{
				File: "test/money_test.dart",
				Kind: entities.KindFunc,
				Name: "main",
			},
			Span:   entities.Span{Start: 0, End: 14},
			IsTest: true,
		},
	}

	pack := NewRulePack(nil)

	if _, err := pack.Edges(context.Background(), snap, decls); err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if _, err := pack.Anchors(context.Background(), snap, decls); err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	if _, err := pack.Atoms(context.Background(), snap, decls); err != nil {
		t.Fatalf("Atoms() error = %v", err)
	}
}

func TestGoScope(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		key  entities.DeclKey
		want bool
	}{
		{"a Go type", entities.DeclKey{File: "a/b.go", Kind: entities.KindType, Name: "B"}, true},
		{
			"a go.mod chunk",
			entities.DeclKey{File: "go.mod", Kind: entities.KindOther, Name: "module"},
			true,
		},
		{
			"a query chunk",
			entities.DeclKey{File: "q/x.sql", Kind: entities.KindOther, Name: "Get"},
			true,
		},
		{
			"a Dart class",
			entities.DeclKey{File: "lib/a.dart", Kind: entities.KindType, Name: "A"},
			false,
		},
		{
			"a Dart function",
			entities.DeclKey{File: "lib/a.dart", Kind: entities.KindFunc, Name: "f"},
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := goScope([]entities.Decl{{Key: testCase.key}})
			if (len(got) == 1) != testCase.want {
				t.Fatalf("goScope kept %d of 1, want kept = %v", len(got), testCase.want)
			}
		})
	}
}

func TestRulePackReportsNoPartialPassBeforeALoad(t *testing.T) {
	t.Parallel()

	if got := NewRulePack(nil).Partial(); len(got) != 0 {
		t.Fatalf("Partial() = %+v, want none before any type-check", got)
	}

	if got := (RulePack{}).Partial(); len(got) != 0 {
		t.Fatalf("zero RulePack Partial() = %+v, want none", got)
	}
}

func TestNewRulePackWithSharesTheSource(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		files map[string]string
	}{
		{
			"a call between two functions",
			map[string]string{"a.go": "package fixture\n\nfunc One() {}\n\nfunc Run() { One() }\n"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, testCase.files)
			source := typed.NewSource(nil)

			before, err := source.Packages(context.Background(), snap)
			if err != nil || len(before) == 0 {
				t.Fatalf("Packages() = %v, %v", before, err)
			}

			pack := NewRulePackWith(source)
			if pack.source != source {
				t.Fatal("NewRulePackWith() did not keep the source it was given")
			}

			if _, err = pack.Edges(context.Background(), snap, decls); err != nil {
				t.Fatalf("Edges() error = %v", err)
			}

			after, err := source.Packages(context.Background(), snap)
			if err != nil || after[0] != before[0] {
				t.Fatalf("the rules loaded the packages again: %v", err)
			}
		})
	}
}

func orderedFixture(
	t *testing.T,
	files map[string]string,
	touches []entities.SourceTouch,
	extra func([]entities.Decl, []entities.Edge) ([]entities.Decl, []entities.Edge),
) (entities.Graph, entities.Ordering) {
	t.Helper()

	snap, decls := testkit.Fixture(t, files)
	snap.History = touches

	edges, err := NewRulePack(nil).Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if extra != nil {
		decls, edges = extra(decls, edges)
	}

	graph := entities.NewGraph(decls, edges, nil)

	result, err := ordering.New(ordering.Config{}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() error = %v", err)
	}

	return graph, result
}

func TestRulePackOrdersWhatSatisfiesWithItsReaders(t *testing.T) {
	t.Parallel()

	_, result := orderedFixture(t, map[string]string{
		"ordering/err.go":       "package ordering\n\ntype Unresolved struct{}\n",
		"ordering/errmethod.go": "package ordering\n\nfunc (*Unresolved) Error() string { return \"\" }\n",
		"ordering/collect.go": "package ordering\n\nfunc collect() []error {\n\tvar errs []error\n\n" +
			"\treturn append(errs, &Unresolved{})\n}\n",
		"main.go": "package main\n\nfunc main() {}\n",
		"run.go":  "package main\n\nfunc run() {}\n",
	}, []entities.SourceTouch{
		{Position: 1, Touched: []string{"ordering/collect.go", "ordering/err.go", "run.go"}},
		{Position: 5, Touched: []string{"ordering/errmethod.go"}},
		{Position: 6, Touched: []string{"main.go"}},
	}, nil)

	testCases := []struct {
		name     string
		provider entities.DeclKey
		reader   entities.DeclKey
	}{
		{
			"the method that satisfies error is born with the append that converts to it",
			testkit.DeclKey("ordering/errmethod.go", entities.KindMethod, "Unresolved.Error"),
			testkit.DeclKey("ordering/collect.go", entities.KindFunc, "collect"),
		},
		{
			"a declaration of package main is ordered with its func main",
			testkit.DeclKey("main.go", entities.KindFunc, "main"),
			testkit.DeclKey("run.go", entities.KindFunc, "run"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider, reader := result.Of(testCase.provider), result.Of(testCase.reader)
			if provider == entities.NoPosition || provider != reader {
				t.Fatalf(
					"%v at %d is not born with %v at %d",
					testCase.provider, provider, testCase.reader, reader,
				)
			}
		})
	}
}
