package ordering

import (
	"errors"
	"fmt"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestEngineDelay(t *testing.T) {
	t.Parallel()

	inserted := entities.DeclKey{File: "new.go"}
	original := entities.DeclKey{File: "old.go"}

	testCases := []struct {
		name         string
		dependent    entities.DeclKey
		provider     entities.DeclKey
		insertedDeps map[entities.DeclKey]bool
		want         entities.Position
	}{
		{
			"inserted reads original, waits a slot",
			inserted,
			original,
			map[entities.DeclKey]bool{inserted: true},
			1,
		},
		{
			"inserted reads inserted, no wait",
			inserted,
			original,
			map[entities.DeclKey]bool{inserted: true, original: true},
			0,
		},
		{"original reads original, no wait", original, original, nil, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			eng := &engine{inserted: testCase.insertedDeps}

			if got := eng.delay(testCase.dependent, testCase.provider); got != testCase.want {
				t.Fatalf("delay() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestEngineRaise(t *testing.T) {
	t.Parallel()

	key := entities.DeclKey{File: "a.go"}

	testCases := []struct {
		name    string
		current entities.Position
		bound   entities.Position
		limit   entities.Position
		want    entities.Position
		changed bool
	}{
		{"a higher bound raises the assignment", 2, 5, 10, 5, true},
		{"a lower bound never moves it earlier", 5, 2, 10, 5, false},
		{"the bound is capped at limit", 2, 50, 10, 10, true},
		{"equal to the current value is not a change", 5, 5, 10, 5, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			eng := &engine{
				assign: map[entities.DeclKey]entities.Position{key: testCase.current},
				limit:  testCase.limit,
			}

			changed := eng.raise(key, testCase.bound)
			if changed != testCase.changed {
				t.Fatalf("raise() changed = %v, want %v", changed, testCase.changed)
			}

			if got := eng.assign[key]; got != testCase.want {
				t.Fatalf("assign = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestEngineConvergeReturnsErrorOnRunaway(t *testing.T) {
	t.Parallel()

	// A chain root <- L1 <- L2 <- ... <- L6, where L1 is nearest the root
	// and L6 is the leaf. Files are named so the alphabetical decl order
	// (leaf-first: L6, L5, ..., L1, root) processes each declaration before
	// the provider it reads has been touched this round, and edges are
	// registered leaf-edge-first so the same-round edge sweep is equally
	// starved. Only root carries history, so only the root-L1 hop grows the
	// position; propagating that growth six hops out takes more than one
	// round.
	const depth = 6

	root := entities.DeclKey{File: "z.go", Name: "root"}
	decls := []entities.Decl{{Key: root, Unit: "chain"}}

	var edges []entities.Edge

	provider := root

	for i := 1; i <= depth; i++ {
		file := fmt.Sprintf("l%c.go", 'a'+depth-i)
		key := entities.DeclKey{File: file, Name: fmt.Sprintf("L%d", i)}
		decls = append(decls, entities.Decl{Key: key, Unit: "chain"})
		edges = append(
			[]entities.Edge{{Dependent: key, Provider: provider, Reason: entities.ReasonReference}},
			edges...,
		)

		provider = key
	}

	graph := entities.NewGraph(decls, edges, nil)
	snap := entities.Snapshot{
		History: []entities.SourceTouch{{Position: 0, Touched: []string{root.File}}},
	}

	eng := newEngine(graph, snap, 100, entities.Layers{}, entities.Features{})

	_, err := eng.converge(1)
	if !errors.Is(err, apperr.ErrIncomplete) {
		t.Fatalf("converge() err = %v, want wrapping %v", err, apperr.ErrIncomplete)
	}
}

func TestEngineRecomputeUnitFloorUsesExternalConsumersOnly(t *testing.T) {
	t.Parallel()

	provider := entities.DeclKey{File: "repo.go", Name: "New"}
	internal := entities.DeclKey{File: "repo.go", Name: "helper"}
	external := entities.DeclKey{File: "usecase.go", Name: "Create"}

	graph := entities.NewGraph(
		[]entities.Decl{
			{Key: provider, Unit: "repo"},
			{Key: internal, Unit: "repo"},
			{Key: external, Unit: "usecase"},
		},
		[]entities.Edge{
			{Dependent: internal, Provider: provider, Reason: entities.ReasonReference},
			{Dependent: external, Provider: provider, Reason: entities.ReasonReference},
		},
		nil,
	)

	eng := &engine{
		graph: graph,
		decls: graph.Decls(),
		assign: map[entities.DeclKey]entities.Position{
			provider: 0, internal: 1, external: 487,
		},
	}

	eng.recomputeUnitFloor()

	if got := eng.unitFloor["repo"]; got != 487 {
		t.Fatalf("unitFloor(repo) = %d, want 487 (external reader only)", got)
	}
}

// TestExternalFloorAgainstInsertionDelay pins the pair of rules that used to
// have no fixed point together. The external-reader floor pulls a provider
// forward to its first outside reader, while the insertion delay pushes an
// inserted reader one slot past its provider. Reading the reader's position
// back into the provider's frame is what makes them settle instead of both
// climbing to the limit and being reported as unresolvable.
func TestExternalFloorAgainstInsertionDelay(t *testing.T) {
	t.Parallel()

	provider := entities.Decl{
		Key:  entities.DeclKey{File: "old/lib.go", Kind: entities.KindFunc, Name: "Provide"},
		Unit: "old",
	}

	testCases := []struct {
		name          string
		dependentFile string
		history       []entities.SourceTouch
		wantProvider  entities.Position
		wantDependent entities.Position
	}{
		{
			"an inserted reader across a unit boundary settles one slot later",
			"new/app.go",
			[]entities.SourceTouch{{Position: 0, Touched: []string{"old/lib.go"}}},
			0,
			1,
		},
		{
			"a reader the history already knows shares the provider's position",
			"other/app.go",
			[]entities.SourceTouch{
				{Position: 0, Touched: []string{"old/lib.go", "other/app.go"}},
			},
			0,
			0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dependent := entities.Decl{
				Key: entities.DeclKey{
					File: testCase.dependentFile,
					Kind: entities.KindFunc,
					Name: "Use",
				},
				Unit: "consumer",
			}

			graph := entities.NewGraph(
				[]entities.Decl{provider, dependent},
				[]entities.Edge{{
					Dependent: dependent.Key,
					Provider:  provider.Key,
					Reason:    entities.ReasonReference,
				}},
				nil,
			)

			got, err := New(Config{Slack: 8, RoundLimit: 2000}).Order(
				graph,
				entities.Snapshot{
					Files:   []string{"old/lib.go", testCase.dependentFile},
					History: testCase.history,
				},
			)
			if err != nil {
				t.Fatalf("Order() = %v", err)
			}

			if at := got.Of(provider.Key); at != testCase.wantProvider {
				t.Fatalf("provider at %d, want %d", at, testCase.wantProvider)
			}

			if at := got.Of(dependent.Key); at != testCase.wantDependent {
				t.Fatalf("dependent at %d, want %d", at, testCase.wantDependent)
			}
		})
	}
}

// TestDanglingEdgeInventsNoPosition pins a case the package once assumed away.
// A rule pack can name a provider it inferred from a comment header or a table
// name that matches no declaration. Reading that missing side as position zero
// both invents a placement for a key nothing declares and pins whatever waits
// on it to the very start of the history.
func TestDanglingEdgeInventsNoPosition(t *testing.T) {
	t.Parallel()

	present := entities.Decl{
		Key:  entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Real"},
		Unit: "a",
	}
	ghost := entities.DeclKey{File: "nowhere.go", Kind: entities.KindType, Name: "Ghost"}

	graph := entities.NewGraph(
		[]entities.Decl{present},
		[]entities.Edge{
			{Dependent: present.Key, Provider: ghost, Reason: entities.ReasonMirrors},
		},
		nil,
	)

	got, err := New(Config{}).Order(graph, entities.Snapshot{
		Files:   []string{"a.go"},
		History: []entities.SourceTouch{{Position: 3, Touched: []string{"a.go"}}},
	})
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	if _, placed := got.At[ghost]; placed {
		t.Fatalf("a key no declaration carries was given position %d", got.At[ghost])
	}

	if len(got.At) != 1 {
		t.Fatalf("At holds %d entries, want only the one present declaration", len(got.At))
	}

	if at := got.Of(present.Key); at != 3 {
		t.Fatalf("the present declaration landed at %d, want its own base of 3", at)
	}
}
