package emitting

import (
	"fmt"
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

func TestRisksForOrdersChunksWhoseTextIsEqual(t *testing.T) {
	t.Parallel()

	target := key("z.go", "Target")

	testCases := []struct {
		name      string
		pairs     int
		isReverse bool
	}{
		{"one pair arriving in order", 1, false},
		{"one pair arriving reversed", 1, true},
		{"many pairs arriving in order", 64, false},
		{"many pairs arriving reversed", 64, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			chunks := make([]entities.DeclKey, 0, 2*testCase.pairs)
			edges := make([]entities.Edge, 0, 2*testCase.pairs)

			for index := range testCase.pairs {
				file := fmt.Sprintf("p%03d", index)
				chunks = append(
					chunks,
					entities.DeclKey{File: file + ":func", Kind: entities.KindType, Name: "T"},
					entities.DeclKey{File: file, Kind: entities.KindFunc, Name: "type:T"},
				)
			}

			for _, chunk := range chunks {
				edges = append(edges, entities.Edge{
					Dependent: chunk, Provider: target, Reason: entities.ReasonReference,
				})
			}

			if testCase.isReverse {
				slices.Reverse(chunks)
			}

			graph := entities.NewGraph(nil, edges, nil)
			planned := entities.PlannedCommit{Position: 0, Decls: chunks}
			positions := map[entities.DeclKey]entities.Position{target: 1}

			got := risksFor(planned, graph, positions)
			if len(got) != len(chunks) {
				t.Fatalf("risksFor() returned %d risks, want %d", len(got), len(chunks))
			}

			isOrdered := slices.IsSortedFunc(got, func(left, right Risk) int {
				return entities.CompareKeys(left.Chunk, right.Chunk)
			})
			if !isOrdered {
				t.Fatalf("risks are not ordered by chunk key: %+v", got)
			}
		})
	}
}

func TestRisksForIgnoresAnInterfaceATypeOnlyFills(t *testing.T) {
	t.Parallel()

	impl := key("a.go", "Impl")
	target := key("b.go", "Target")

	testCases := []struct {
		name        string
		reason      entities.EdgeReason
		isInterface bool
		want        []Risk
	}{
		{"filling an interface born later compiles", entities.ReasonImplements, true, nil},
		{
			"passing a value on needs the method that fills it",
			entities.ReasonImplements,
			false,
			[]Risk{{Chunk: impl, Target: target, BornAt: 1}},
		},
		{
			"a reference to the interface does not compile",
			entities.ReasonReference,
			true,
			[]Risk{{Chunk: impl, Target: target, BornAt: 1}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(
				[]entities.Decl{{Key: target, IsInterface: testCase.isInterface}},
				[]entities.Edge{{Dependent: impl, Provider: target, Reason: testCase.reason}},
				nil,
			)
			planned := entities.PlannedCommit{Position: 0, Decls: []entities.DeclKey{impl}}
			positions := map[entities.DeclKey]entities.Position{impl: 0, target: 1}

			if got := risksFor(planned, graph, positions); !slices.Equal(got, testCase.want) {
				t.Fatalf("risksFor() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
