package emitting

import (
	"errors"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestTopoSort(t *testing.T) {
	t.Parallel()

	a := key("a.go", "A")
	b := key("a.go", "B")
	c := key("a.go", "C")

	testCases := []struct {
		name  string
		decls []entities.DeclKey
		edges []entities.Edge
		want  []entities.DeclKey
	}{
		{
			"no edges keeps the plan's own order",
			[]entities.DeclKey{a, b, c},
			nil,
			[]entities.DeclKey{a, b, c},
		},
		{
			"a dependent moves after its in-commit provider",
			[]entities.DeclKey{a, b},
			[]entities.Edge{{Dependent: a, Provider: b, Reason: entities.ReasonReference}},
			[]entities.DeclKey{b, a},
		},
		{
			"an edge to a decl outside this commit is ignored",
			[]entities.DeclKey{a, b},
			[]entities.Edge{{Dependent: a, Provider: c, Reason: entities.ReasonReference}},
			[]entities.DeclKey{a, b},
		},
		{
			"a self-reference does not block its own decl",
			[]entities.DeclKey{a},
			[]entities.Edge{{Dependent: a, Provider: a, Reason: entities.ReasonReference}},
			[]entities.DeclKey{a},
		},
		{
			// simplification: a cycle among one commit's own declarations
			// should already be resolved upstream; topoSort falls back to
			// the plan's own order rather than dropping a declaration.
			"a cycle falls back to the plan's own order",
			[]entities.DeclKey{a, b},
			[]entities.Edge{
				{Dependent: a, Provider: b, Reason: entities.ReasonReference},
				{Dependent: b, Provider: a, Reason: entities.ReasonReference},
			},
			[]entities.DeclKey{a, b},
		},
		{
			// Two edges naming the same provider must not double-count: an
			// undeduped in-degree would never reach zero for A, and A would
			// wrongly fall back to plan order as if it were part of a cycle.
			"duplicate edges to the same provider do not inflate in-degree",
			[]entities.DeclKey{a, b},
			[]entities.Edge{
				{Dependent: a, Provider: b, Reason: entities.ReasonReference},
				{Dependent: a, Provider: b, Reason: entities.ReasonMethodSet},
			},
			[]entities.DeclKey{b, a},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(nil, testCase.edges, nil)

			got := topoSort(testCase.decls, graph)
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("topoSort() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestChunksForResolvesSpanAndUnit(t *testing.T) {
	t.Parallel()

	a := key("a.go", "A")
	graph := entities.NewGraph(
		[]entities.Decl{decl(a, entities.Span{Start: 1, End: 9})},
		nil,
		nil,
	)

	planned := entities.PlannedCommit{Decls: []entities.DeclKey{a}}

	chunks, err := chunksFor(planned, graph)
	if err != nil {
		t.Fatalf("chunksFor() = %v", err)
	}

	want := Chunk{Key: a, Span: entities.Span{Start: 1, End: 9}, Unit: "a.go"}
	if chunks[0] != want {
		t.Fatalf("chunksFor() = %+v, want %+v", chunks[0], want)
	}
}

func TestChunksForRefusesADeclarationTheGraphNeverSaw(t *testing.T) {
	t.Parallel()

	planned := entities.PlannedCommit{Decls: []entities.DeclKey{key("a.go", "Ghost")}}

	_, err := chunksFor(planned, entities.NewGraph(nil, nil, nil))
	if !errors.Is(err, apperr.ErrIncomplete) {
		t.Fatalf("chunksFor() = %v, want %v", err, apperr.ErrIncomplete)
	}
}
