package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
)

// featureRules decides the order of the features: what must precede what, where
// each first appeared, its product and its cost.
type featureRules struct {
	needs map[FeatureID]map[FeatureID]bool
	waits map[FeatureID]map[FeatureID]bool
	at    map[FeatureID]entities.Position
	group map[FeatureID]FeatureID
	cost  map[FeatureID]int
}

// productReaders counts, per product, the other products whose features read
// its module. The product many others read is the core, and what a core shares
// is born inside its own arc only if it comes first: behind a small reader, that
// reader pulls the shared half forward and splits the core in two. Cost orders
// the products nobody reads, and breaks a tie.
func productReaders(
	waits map[FeatureID]map[FeatureID]bool,
	product func(FeatureID) FeatureID,
) map[FeatureID]int {
	seen := map[FeatureID]map[FeatureID]bool{}

	for feature, roots := range waits {
		for root := range roots {
			reader, read := product(feature), product(root)
			if reader == read {
				continue
			}

			if seen[read] == nil {
				seen[read] = map[FeatureID]bool{}
			}

			seen[read][reader] = true
		}
	}

	out := make(map[FeatureID]int, len(seen))
	for read, readersOf := range seen {
		out[read] = len(readersOf)
	}

	return out
}

// sortFeatures returns the build order and the features that need each other.
// A cycle is returned, never silently broken. A product is built as a unit, its
// root first; a feature also waits for the roots of the modules it reads,
// unless that would leave nothing to build.
func sortFeatures(
	features []FeatureID,
	rules featureRules,
) (sorted []FeatureID, cyclic []FeatureID) {
	needs, waits, at, group, cost := rules.needs, rules.waits, rules.at, rules.group, rules.cost
	product := func(feature FeatureID) FeatureID { return owners.ProductOf(group, feature) }

	readers := productReaders(waits, product)
	pending := slices.Clone(features)
	slices.SortFunc(pending, func(a, b FeatureID) int {
		left, right := product(a), product(b)
		if byReaders := cmp.Compare(readers[right], readers[left]); byReaders != 0 {
			return byReaders
		}

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
