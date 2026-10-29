package sequence

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

type FeatureRules struct {
	Needs map[reach.FeatureID]map[reach.FeatureID]bool
	Waits map[reach.FeatureID]map[reach.FeatureID]bool
	At    map[reach.FeatureID]entities.Position
	Group map[reach.FeatureID]reach.FeatureID
	Cost  map[reach.FeatureID]int
}

func productReaders(
	waits map[reach.FeatureID]map[reach.FeatureID]bool,
	product func(reach.FeatureID) reach.FeatureID,
) map[reach.FeatureID]int {
	seen := map[reach.FeatureID]map[reach.FeatureID]bool{}

	// The product many others read is the core, and what a core shares is born
	// inside its own arc only if it comes first: behind a small reader, that
	// reader pulls the shared half forward and splits the core in two.
	for feature, roots := range waits {
		for root := range roots {
			reader, read := product(feature), product(root)
			if reader == read {
				continue
			}

			if seen[read] == nil {
				seen[read] = map[reach.FeatureID]bool{}
			}

			seen[read][reader] = true
		}
	}

	out := make(map[reach.FeatureID]int, len(seen))
	for read, readersOf := range seen {
		out[read] = len(readersOf)
	}

	return out
}

func SortFeatures(
	features []reach.FeatureID,
	rules FeatureRules,
) (sorted []reach.FeatureID, cyclic []reach.FeatureID) {
	needs, waits, at, group, cost := rules.Needs, rules.Waits, rules.At, rules.Group, rules.Cost
	product := func(feature reach.FeatureID) reach.FeatureID { return owners.ProductOf(group, feature) }

	readers := productReaders(waits, product)
	pending := slices.Clone(features)
	// A product is built as a unit, its root first.
	slices.SortFunc(pending, func(first, second reach.FeatureID) int {
		left, right := product(first), product(second)
		if byReaders := cmp.Compare(readers[right], readers[left]); byReaders != 0 {
			return byReaders
		}

		if byCost := cmp.Compare(cost[left], cost[right]); byCost != 0 {
			return byCost
		}

		if byRoot := cmp.Compare(left, right); byRoot != 0 {
			return byRoot
		}

		if byRank := cmp.Compare(rankIn(first, left), rankIn(second, right)); byRank != 0 {
			return byRank
		}

		if byCost := cmp.Compare(cost[first], cost[second]); byCost != 0 {
			return byCost
		}

		if byPlace := cmp.Compare(at[first], at[second]); byPlace != 0 {
			return byPlace
		}

		return cmp.Compare(first, second)
	})

	done := map[reach.FeatureID]bool{}
	last := reach.FeatureID("")

	for len(pending) > 0 {
		next := nextFeature(pending, readiness{needs, waits, done}, product, last)
		// A feature also waits for the roots of the modules it reads, unless that
		// would leave nothing to build.
		if next < 0 {
			next = nextFeature(pending, readiness{needs, nil, done}, product, last)
		}

		if next < 0 {
			// A cycle is returned, never silently broken.
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

func rankIn(feature, root reach.FeatureID) int {
	if feature == root {
		return 0
	}

	return 1
}
