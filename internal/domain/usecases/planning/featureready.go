package planning

// readiness is what has to be built before a feature may be.
type readiness struct {
	needs map[FeatureID]map[FeatureID]bool
	waits map[FeatureID]map[FeatureID]bool
	done  map[FeatureID]bool
}

func (r readiness) ready(feature FeatureID) bool {
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
	pending []FeatureID,
	state readiness,
	product func(FeatureID) FeatureID,
	last FeatureID,
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
