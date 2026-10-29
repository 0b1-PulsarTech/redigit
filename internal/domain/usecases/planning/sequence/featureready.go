package sequence

import "github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"

// readiness is what has to be built before a feature may be.
type readiness struct {
	needs map[reach.FeatureID]map[reach.FeatureID]bool
	waits map[reach.FeatureID]map[reach.FeatureID]bool
	done  map[reach.FeatureID]bool
}

func (r readiness) ready(feature reach.FeatureID) bool {
	for other := range r.needs[feature] {
		if other != feature && !r.done[other] {
			return false
		}
	}

	for other := range r.waits[feature] {
		if !r.done[other] {
			return false
		}
	}

	return true
}

// nextFeature is the index of the first ready feature in the last product, else
// of the first ready feature; pending is already in build order.
func nextFeature(
	pending []reach.FeatureID,
	state readiness,
	product func(reach.FeatureID) reach.FeatureID,
	last reach.FeatureID,
) int {
	first := -1

	for index, feature := range pending {
		if !state.ready(feature) {
			continue
		}

		if last != "" && product(feature) == last {
			return index
		}

		if first < 0 {
			first = index
		}
	}

	return first
}
