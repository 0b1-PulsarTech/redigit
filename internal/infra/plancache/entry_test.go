package plancache

import (
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

func declKey(name string) entities.DeclKey {
	return entities.DeclKey{File: name + ".go", Kind: entities.KindFunc, Name: name}
}

func sampleResult() indexing.Result {
	decls := []entities.Decl{
		{
			Key: declKey("B"), Unit: "pkg#b", Span: entities.Span{Start: 4, End: 40},
			Selector: entities.Location{File: "B.go", Line: 2, Char: 5},
			IsTest:   true, IsInterface: true, Atom: "atom", GroupSize: 3, Lines: 9,
			Frame: 2, Generated: true,
		},
		{Key: declKey("A"), Unit: "pkg#a", Lines: 3},
	}
	edges := []entities.Edge{
		{Dependent: declKey("B"), Provider: declKey("A"), Reason: entities.ReasonReference},
		{Dependent: declKey("A"), Provider: declKey("B"), Reason: entities.ReasonImplements},
	}
	anchors := []entities.Anchor{
		{Decl: declKey("B"), To: declKey("A"), Reason: entities.ReasonReference},
	}

	return indexing.Result{
		Graph:   entities.NewGraph(decls, edges, anchors),
		Blind:   []indexing.Blind{{Decl: declKey("B"), Query: "references", Reason: "no answer"}},
		Partial: []indexing.Partial{{Broken: 2, Of: 9, Tags: []string{"integration"}}},
	}
}

func sampleEntry() Entry {
	entry := Pack(sampleResult())
	entry.Ordering = entities.Ordering{
		At: map[entities.DeclKey]entities.Position{declKey("A"): 0, declKey("B"): 1},
		Cycles: []entities.Cycle{{
			Members: []entities.DeclKey{declKey("A"), declKey("B")},
			Reasons: []entities.EdgeReason{entities.ReasonReference},
		}},
		Rounds: 3,
	}
	entry.Commit = "0123abc"
	entry.OrderNote = "a cycle remains"
	entry.Plan = entities.Plan{
		Commits: []entities.PlannedCommit{{
			Position: 1, Subject: "DRAFT: add", Shape: entities.ShapeFeature, Feature: "f",
			Decls: []entities.DeclKey{declKey("A")}, Reason: "why", IsWIP: true,
		}},
		Unplaced: []entities.DeclKey{declKey("B")},
	}
	entry.Unseen = []auditing.Unseen{
		{File: "x.go", Constraint: "//go:build x", Tags: []string{"x"}},
	}
	entry.Unclaimed = []auditing.Unclaimed{{File: "y.bin"}}

	return entry
}

func TestPackResultRoundTrip(t *testing.T) {
	t.Parallel()

	original := sampleResult()
	got := Pack(original).Result()

	if !reflect.DeepEqual(got, original) {
		t.Fatalf("Result() = %+v, want %+v", got, original)
	}
}

func TestPackIsDeterministic(t *testing.T) {
	t.Parallel()

	first := Pack(sampleResult())

	for range 20 {
		if !reflect.DeepEqual(Pack(sampleResult()).Decls, first.Decls) {
			t.Fatal("Pack() ordered the declarations differently between runs")
		}
	}

	if first.Decls[0].Key.Name != "A" {
		t.Fatalf("Pack() first decl = %q, want A (sorted by key)", first.Decls[0].Key.Name)
	}
}

func TestPackKeepsTheGraphDeclOrder(t *testing.T) {
	t.Parallel()

	nested := entities.DeclKey{File: "x", Kind: entities.KindFunc, Name: "F"}
	sibling := entities.DeclKey{File: "x.go", Kind: entities.KindFunc, Name: "F"}
	other := entities.DeclKey{File: "a.go", Kind: entities.KindType, Name: "T"}

	testCases := []struct {
		name  string
		decls []entities.Decl
	}{
		{"a file that prefixes another", []entities.Decl{{Key: nested}, {Key: sibling}}},
		{"unrelated files", []entities.Decl{{Key: sibling}, {Key: other}}},
		{
			"three files in any arrival",
			[]entities.Decl{{Key: sibling}, {Key: nested}, {Key: other}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(testCase.decls, nil, nil)

			got := Pack(indexing.Result{Graph: graph}).Decls
			if !reflect.DeepEqual(got, graph.Decls()) {
				t.Fatalf("Pack().Decls = %v, want %v", got, graph.Decls())
			}
		})
	}
}

func TestSampleEntryFillsEveryField(t *testing.T) {
	t.Parallel()

	entry := sampleEntry()
	fields := reflect.ValueOf(entry)

	for idx := range fields.NumField() {
		if fields.Field(idx).IsZero() {
			t.Errorf("sampleEntry() leaves Entry.%s empty", fields.Type().Field(idx).Name)
		}
	}

	decl := reflect.ValueOf(entry.Decls[1])
	for idx := range decl.NumField() {
		if decl.Field(idx).IsZero() {
			t.Errorf("sampleEntry() leaves Decl.%s empty", decl.Type().Field(idx).Name)
		}
	}
}
