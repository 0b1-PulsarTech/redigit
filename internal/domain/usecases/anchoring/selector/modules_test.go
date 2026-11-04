package selector

import (
	"maps"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestModuleDirs(t *testing.T) {
	t.Parallel()

	edge := func(reason entities.EdgeReason, provider string) entities.Edge {
		return entities.Edge{
			Dependent: entities.DeclKey{File: "x.go", Kind: entities.KindFunc, Name: "X"},
			Provider:  entities.DeclKey{File: provider, Kind: entities.KindOther, Name: "m"},
			Reason:    reason,
		}
	}

	testCases := []struct {
		name  string
		edges []entities.Edge
		want  []string
	}{
		{name: "no edge names no module"},
		{
			name: "a root and a nested module",
			edges: []entities.Edge{
				edge(entities.ReasonModule, "go.mod"), edge(entities.ReasonModule, "aaext/go.mod"),
			},
			want: []string{".", "aaext"},
		},
		{
			name:  "another reason is not a module",
			edges: []entities.Edge{edge(entities.ReasonReference, "lib/go.mod")},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := slices.Sorted(maps.Keys(ModuleDirs(entities.NewGraph(nil, testCase.edges, nil))))
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("ModuleDirs() = %v, want %v", got, testCase.want)
			}
		})
	}
}
