package ordering

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestFindCycles(t *testing.T) {
	t.Parallel()

	a := entities.DeclKey{File: "a.go", Name: "A"}
	b := entities.DeclKey{File: "b.go", Name: "B"}
	c := entities.DeclKey{File: "c.go", Name: "C"}

	testCases := []struct {
		name        string
		decls       []entities.Decl
		edges       []entities.Edge
		wantCycles  int
		wantMembers []entities.DeclKey
		wantReasons []entities.EdgeReason
	}{
		{
			"a two-node cycle is named with its reasons, not silently ordered",
			[]entities.Decl{{Key: a}, {Key: b}},
			[]entities.Edge{
				{Dependent: a, Provider: b, Reason: entities.ReasonReference},
				{Dependent: b, Provider: a, Reason: entities.ReasonImplements},
			},
			1,
			[]entities.DeclKey{a, b},
			[]entities.EdgeReason{entities.ReasonImplements, entities.ReasonReference},
		},
		{
			"a plain chain holds no cycle",
			[]entities.Decl{{Key: a}, {Key: b}, {Key: c}},
			[]entities.Edge{
				{Dependent: a, Provider: b, Reason: entities.ReasonReference},
				{Dependent: b, Provider: c, Reason: entities.ReasonReference},
			},
			0,
			nil,
			nil,
		},
		{
			"a self-loop is not a cycle of size greater than one",
			[]entities.Decl{{Key: a}},
			[]entities.Edge{{Dependent: a, Provider: a, Reason: entities.ReasonEncloses}},
			0,
			nil,
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(testCase.decls, testCase.edges, nil)
			cycles := findCycles(graph)

			if len(cycles) != testCase.wantCycles {
				t.Fatalf("findCycles() = %d cycles, want %d", len(cycles), testCase.wantCycles)
			}

			if testCase.wantCycles == 0 {
				return
			}

			if !slices.Equal(cycles[0].Members, testCase.wantMembers) {
				t.Fatalf("Members = %v, want %v", cycles[0].Members, testCase.wantMembers)
			}

			if !slices.Equal(cycles[0].Reasons, testCase.wantReasons) {
				t.Fatalf("Reasons = %v, want %v", cycles[0].Reasons, testCase.wantReasons)
			}
		})
	}
}
