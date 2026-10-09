package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSortFeaturesBuildsAProductAsAUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		features []FeatureID
		waits    map[FeatureID]map[FeatureID]bool
		group    map[FeatureID]FeatureID
		cost     map[FeatureID]int
		want     []FeatureID
	}{
		{
			"the root opens its product and the tiers follow before the next product",
			[]FeatureID{"a", "a/routes", "a/repo", "b", "b/routes"},
			nil,
			map[FeatureID]FeatureID{"a/routes": "a", "a/repo": "a", "b/routes": "b"},
			map[FeatureID]int{"a": 1, "b": 2, "a/routes": 9, "a/repo": 9, "b/routes": 9},
			[]FeatureID{"a", "a/repo", "a/routes", "b", "b/routes"},
		},
		{
			"a tier that waits for another root lets the product's next tier go first",
			[]FeatureID{"a", "a/routes", "a/repo", "b"},
			map[FeatureID]map[FeatureID]bool{"a/routes": {"b": true}},
			map[FeatureID]FeatureID{"a/routes": "a", "a/repo": "a"},
			map[FeatureID]int{"a": 1, "b": 5, "a/routes": 2, "a/repo": 3},
			[]FeatureID{"a", "a/repo", "b", "a/routes"},
		},
		{
			"waits that would leave nothing to build are dropped",
			[]FeatureID{"a", "b"},
			map[FeatureID]map[FeatureID]bool{"a": {"b": true}, "b": {"a": true}},
			nil,
			nil,
			[]FeatureID{"a", "b"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, cyclic := sortFeatures(
				testCase.features, nil, testCase.waits, nil, testCase.group, testCase.cost,
			)
			if len(cyclic) != 0 || !slices.Equal(got, testCase.want) {
				t.Fatalf("sortFeatures() = %v (cyclic %v), want %v", got, cyclic, testCase.want)
			}
		})
	}
}

func TestReadsRoots(t *testing.T) {
	t.Parallel()

	manifest := decl("crm/go.mod", "module", "crm")
	port := decl("crm/port.go", "Port", "crm")

	graph := entities.NewGraph(
		[]entities.Decl{manifest, port},
		[]entities.Edge{
			{Dependent: port.Key, Provider: manifest.Key, Reason: entities.ReasonModule},
		},
		nil,
	)
	reached := map[entities.DeclKey]map[FeatureID]bool{
		port.Key: readers("crm", "routes"),
	}

	got := readsRoots(graph, reached, map[string]FeatureID{"crm": "crm"})

	if !got["routes"]["crm"] || len(got["crm"]) != 0 {
		t.Fatalf("readsRoots() = %v, want routes to wait for crm and crm for none", got)
	}
}
