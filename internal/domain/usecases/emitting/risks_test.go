package emitting

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestRisksForPosition(t *testing.T) {
	t.Parallel()

	a := key("a.go", "A")
	b := key("a.go", "B")

	testCases := []struct {
		name      string
		planned   entities.PlannedCommit
		positions map[entities.DeclKey]entities.Position
		want      []Risk
	}{
		{
			"a reference to something born later is a risk",
			entities.PlannedCommit{Position: 0, Decls: []entities.DeclKey{a}},
			map[entities.DeclKey]entities.Position{a: 0, b: 1},
			[]Risk{{Chunk: a, Target: b, BornAt: 1}},
		},
		{
			"a reference to something born in this same commit is not a risk",
			entities.PlannedCommit{Position: 0, Decls: []entities.DeclKey{a}},
			map[entities.DeclKey]entities.Position{a: 0, b: 0},
			nil,
		},
		{
			"a reference to something born earlier is not a risk",
			entities.PlannedCommit{Position: 1, Decls: []entities.DeclKey{a}},
			map[entities.DeclKey]entities.Position{a: 1, b: 0},
			nil,
		},
		{
			"a target this stage cannot place is left out rather than guessed at",
			entities.PlannedCommit{Position: 0, Decls: []entities.DeclKey{a}},
			map[entities.DeclKey]entities.Position{a: 0},
			nil,
		},
	}

	edges := []entities.Edge{{Dependent: a, Provider: b, Reason: entities.ReasonReference}}
	graph := entities.NewGraph(nil, edges, nil)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := risksFor(testCase.planned, graph, testCase.positions)
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("risksFor() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestRisksForAggregation(t *testing.T) {
	t.Parallel()

	a := key("a.go", "A")
	b := key("a.go", "B")
	c := key("a.go", "C")

	testCases := []struct {
		name      string
		planned   entities.PlannedCommit
		edges     []entities.Edge
		positions map[entities.DeclKey]entities.Position
		want      []Risk
	}{
		{
			"two edges to the same forward target report one risk",
			entities.PlannedCommit{Position: 0, Decls: []entities.DeclKey{a}},
			[]entities.Edge{
				{Dependent: a, Provider: b, Reason: entities.ReasonReference},
				{Dependent: a, Provider: b, Reason: entities.ReasonMethodSet},
			},
			map[entities.DeclKey]entities.Position{a: 0, b: 2},
			[]Risk{{Chunk: a, Target: b, BornAt: 2}},
		},
		{
			"every chunk in the commit is checked",
			entities.PlannedCommit{Position: 0, Decls: []entities.DeclKey{a, c}},
			[]entities.Edge{
				{Dependent: a, Provider: b, Reason: entities.ReasonReference},
				{Dependent: c, Provider: b, Reason: entities.ReasonReference},
			},
			map[entities.DeclKey]entities.Position{a: 0, c: 0, b: 3},
			[]Risk{
				{Chunk: a, Target: b, BornAt: 3},
				{Chunk: c, Target: b, BornAt: 3},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(nil, testCase.edges, nil)

			got := risksFor(testCase.planned, graph, testCase.positions)
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("risksFor() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
