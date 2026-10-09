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
//
// Among the features ready to build, the next one is from the same group as the
// last: a module's features arriving back to back are an arc a reader can
// follow, and taking every ready feature of every module in rounds scatters
// each module over the whole history.
func sortFeatures(
	features []FeatureID,
	needs map[FeatureID]map[FeatureID]bool,
	at map[FeatureID]entities.Position,
	group map[FeatureID]string,
	cost map[FeatureID]int,
) (sorted []FeatureID, cyclic []FeatureID) {
	pending := slices.Clone(features)
	slices.SortFunc(pending, func(a, b FeatureID) int {
		if left, right := cost[a], cost[b]; left != right {
			return cmp.Compare(left, right)
		}

		if left, right := at[a], at[b]; left != right {
			return cmp.Compare(left, right)
		}

		return cmp.Compare(a, b)
	})

	done := map[FeatureID]bool{}
	last := ""

	for len(pending) > 0 {
		next := nextFeature(pending, needs, done, group, last)
		if next < 0 {
			return sorted, pending
		}

		feature := pending[next]
		done[feature] = true
		last = group[feature]
		sorted = append(sorted, feature)
		pending = slices.Delete(pending, next, next+1)
	}

	return sorted, nil
}

// nextFeature is the index of the first ready feature in the last group, else
// of the first ready feature; pending is already in chronological order.
func nextFeature(
	pending []FeatureID,
	needs map[FeatureID]map[FeatureID]bool,
	done map[FeatureID]bool,
	group map[FeatureID]string,
	last string,
) int {
	first := -1

	for index, feature := range pending {
		if !readyToBuild(feature, needs, done) {
			continue
		}

		if last != "" && group[feature] == last {
			return index
		}

		if first < 0 {
			first = index
		}
	}

	return first
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
