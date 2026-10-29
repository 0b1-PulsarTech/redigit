package planning

import (
	"slices"
	"testing"
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
			"a product another one reads is built before its reader",
			[]FeatureID{"a", "a/routes", "a/repo", "b"},
			map[FeatureID]map[FeatureID]bool{"a/routes": {"b": true}},
			map[FeatureID]FeatureID{"a/routes": "a", "a/repo": "a"},
			map[FeatureID]int{"a": 1, "b": 5, "a/routes": 2, "a/repo": 3},
			[]FeatureID{"b", "a", "a/routes", "a/repo"},
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
				testCase.features,
				featureRules{waits: testCase.waits, group: testCase.group, cost: testCase.cost},
			)
			if len(cyclic) != 0 || !slices.Equal(got, testCase.want) {
				t.Fatalf("sortFeatures() = %v (cyclic %v), want %v", got, cyclic, testCase.want)
			}
		})
	}
}
