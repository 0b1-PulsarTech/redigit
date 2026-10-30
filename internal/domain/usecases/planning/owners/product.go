package owners

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

const (
	kinReach = 12
	tieScale = 27720
)

func joinProducts(
	reachedBy map[entities.DeclKey]map[reach.FeatureID]bool,
	roots map[reach.FeatureID]bool,
) map[reach.FeatureID]reach.FeatureID {
	tie := map[reach.FeatureID]map[reach.FeatureID]int{}

	// A shared declaration ties its readers with weight 1/(readers-1); a feature
	// joins the root it is tied to most, else stands alone.
	for _, features := range reachedBy {
		// Past kinReach readers a declaration says nothing about which belong
		// together: a helper read by a hundred ties nothing and would chain every
		// module to the next.
		if len(features) < 2 || len(features) > kinReach {
			continue
		}

		// tieScale is lcm(1..kinReach-1): every weight is whole, so sums are exact and
		// neither map order nor key order can split a real tie.
		weight := tieScale / (len(features) - 1)

		for feature := range features {
			for root := range features {
				if feature == root || !roots[root] {
					continue
				}

				if tie[feature] == nil {
					tie[feature] = map[reach.FeatureID]int{}
				}

				tie[feature][root] += weight
			}
		}
	}

	out := map[reach.FeatureID]reach.FeatureID{}

	for feature, ties := range tie {
		if roots[feature] {
			continue
		}

		out[feature] = strongest(ties)
	}

	return out
}

func strongest(ties map[reach.FeatureID]int) reach.FeatureID {
	candidates := make([]reach.FeatureID, 0, len(ties))
	for root := range ties {
		candidates = append(candidates, root)
	}

	return slices.MaxFunc(candidates, func(first, second reach.FeatureID) int {
		if byTie := cmp.Compare(ties[first], ties[second]); byTie != 0 {
			return byTie
		}

		return cmp.Compare(second, first)
	})
}
