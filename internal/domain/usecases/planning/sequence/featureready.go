package sequence

import "github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"

type readiness struct {
	needs map[reach.FeatureID]map[reach.FeatureID]bool
	waits map[reach.FeatureID]map[reach.FeatureID]bool
	done  map[reach.FeatureID]bool
}

func (state readiness) ready(feature reach.FeatureID) bool {
	for other := range state.needs[feature] {
		if other != feature && !state.done[other] {
			return false
		}
	}

	for other := range state.waits[feature] {
		if !state.done[other] {
			return false
		}
	}

	return true
}

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
