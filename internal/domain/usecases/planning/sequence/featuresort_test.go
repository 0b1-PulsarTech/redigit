package sequence

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestSortFeaturesBuildsAProductAsAUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		features []reach.FeatureID
		waits    map[reach.FeatureID]map[reach.FeatureID]bool
		group    map[reach.FeatureID]reach.FeatureID
		cost     map[reach.FeatureID]int
		want     []reach.FeatureID
	}{
		{
			"the root opens its product and the tiers follow before the next product",
			[]reach.FeatureID{"a", "a/routes", "a/repo", "b", "b/routes"},
			nil,
			map[reach.FeatureID]reach.FeatureID{"a/routes": "a", "a/repo": "a", "b/routes": "b"},
			map[reach.FeatureID]int{"a": 1, "b": 2, "a/routes": 9, "a/repo": 9, "b/routes": 9},
			[]reach.FeatureID{"a", "a/repo", "a/routes", "b", "b/routes"},
		},
		{
			"a product another one reads is built before its reader",
			[]reach.FeatureID{"a", "a/routes", "a/repo", "b"},
			map[reach.FeatureID]map[reach.FeatureID]bool{"a/routes": {"b": true}},
			map[reach.FeatureID]reach.FeatureID{"a/routes": "a", "a/repo": "a"},
			map[reach.FeatureID]int{"a": 1, "b": 5, "a/routes": 2, "a/repo": 3},
			[]reach.FeatureID{"b", "a", "a/routes", "a/repo"},
		},
		{
			"waits that would leave nothing to build are dropped",
			[]reach.FeatureID{"a", "b"},
			map[reach.FeatureID]map[reach.FeatureID]bool{"a": {"b": true}, "b": {"a": true}},
			nil,
			nil,
			[]reach.FeatureID{"a", "b"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, cyclic := SortFeatures(
				testCase.features,
				FeatureRules{Waits: testCase.waits, Group: testCase.group, Cost: testCase.cost},
			)
			if len(cyclic) != 0 || !slices.Equal(got, testCase.want) {
				t.Fatalf("sortFeatures() = %v (cyclic %v), want %v", got, cyclic, testCase.want)
			}
		})
	}
}
