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
// A product is built as a unit: its root first, then the features serving it,
// and the products among themselves from the cheapest. When a feature of the
// product cannot be built yet, the next ready one of the same product goes
// first, because a product arriving back to back is an arc a reader can follow,
// and taking every ready feature of every product in rounds scatters each one
// over the whole history.
//
// A feature also waits for the roots of the modules it reads, unless that would
// leave nothing to build.
func sortFeatures(
	features []FeatureID,
	needs map[FeatureID]map[FeatureID]bool,
	waits map[FeatureID]map[FeatureID]bool,
	at map[FeatureID]entities.Position,
	group map[FeatureID]FeatureID,
	cost map[FeatureID]int,
) (sorted []FeatureID, cyclic []FeatureID) {
	product := func(feature FeatureID) FeatureID { return productOf(group, feature) }

	pending := slices.Clone(features)
	slices.SortFunc(pending, func(a, b FeatureID) int {
		left, right := product(a), product(b)
		if byCost := cmp.Compare(cost[left], cost[right]); byCost != 0 {
			return byCost
		}

		if byRoot := cmp.Compare(left, right); byRoot != 0 {
			return byRoot
		}

		if byRank := cmp.Compare(rankIn(a, left), rankIn(b, right)); byRank != 0 {
			return byRank
		}

		if byCost := cmp.Compare(cost[a], cost[b]); byCost != 0 {
			return byCost
		}

		if byPlace := cmp.Compare(at[a], at[b]); byPlace != 0 {
			return byPlace
		}

		return cmp.Compare(a, b)
	})

	done := map[FeatureID]bool{}
	last := FeatureID("")

	for len(pending) > 0 {
		next := nextFeature(pending, readiness{needs, waits, done}, product, last)
		if next < 0 {
			next = nextFeature(pending, readiness{needs, nil, done}, product, last)
		}

		if next < 0 {
			return sorted, pending
		}

		feature := pending[next]
		done[feature] = true
		last = product(feature)
		sorted = append(sorted, feature)
		pending = slices.Delete(pending, next, next+1)
	}

	return sorted, nil
}

// rankIn puts the root of a product before the features that serve it.
func rankIn(feature, root FeatureID) int {
	if feature == root {
		return 0
	}

	return 1
}

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
