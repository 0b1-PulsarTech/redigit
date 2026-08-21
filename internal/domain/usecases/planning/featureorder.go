package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// featureNeeds is which features have to be built before which. A feature needs
// another when any declaration it owns waits on a declaration the other owns.
func featureNeeds(graph entities.Graph, own ownership) map[FeatureID]map[FeatureID]bool {
	needs := map[FeatureID]map[FeatureID]bool{}

	for _, edge := range graph.Edges() {
		from, to := own.owner[edge.Dependent], own.owner[edge.Provider]
		if from == SharedFeature || to == SharedFeature || from == to {
			continue
		}

		if needs[from] == nil {
			needs[from] = map[FeatureID]bool{}
		}

		needs[from][to] = true
	}

	return needs
}

// earliest is the lowest position any of a feature's declarations reached. It
// is the tie-break between features neither of which needs the other, so a
// rebuilt history still follows the original chronology wherever the dependency
// graph leaves the choice open.
func earliest(own ownership, order entities.Ordering) map[FeatureID]entities.Position {
	out := map[FeatureID]entities.Position{}

	for key, feature := range own.owner {
		at := order.Of(key)
		if !at.IsSet() {
			continue
		}

		if current, found := out[feature]; !found || at < current {
			out[feature] = at
		}
	}

	return out
}

// sortFeatures returns the build order and the features that could not be
// ordered because they need each other. A cycle is returned, never silently
// broken: two features that need each other are one feature, or one of the
// edges between them is wrong, and both are answers a reader must see.
func sortFeatures(
	features []FeatureID,
	needs map[FeatureID]map[FeatureID]bool,
	at map[FeatureID]entities.Position,
) (sorted []FeatureID, cyclic []FeatureID) {
	pending := slices.Clone(features)
	slices.SortFunc(pending, func(a, b FeatureID) int {
		if left, right := at[a], at[b]; left != right {
			return cmp.Compare(left, right)
		}

		return cmp.Compare(a, b)
	})

	done := map[FeatureID]bool{}

	for len(pending) > 0 {
		ready := pending[:0:0]
		blocked := pending[:0:0]

		for _, feature := range pending {
			if readyToBuild(feature, needs, done) {
				ready = append(ready, feature)

				continue
			}

			blocked = append(blocked, feature)
		}

		if len(ready) == 0 {
			return sorted, blocked
		}

		for _, feature := range ready {
			done[feature] = true
		}

		sorted = append(sorted, ready...)
		pending = blocked
	}

	return sorted, nil
}

func readyToBuild(
	feature FeatureID,
	needs map[FeatureID]map[FeatureID]bool,
	done map[FeatureID]bool,
) bool {
	for other := range needs[feature] {
		if other != feature && !done[other] {
			return false
		}
	}

	return true
}
