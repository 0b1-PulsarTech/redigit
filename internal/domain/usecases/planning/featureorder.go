package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

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
