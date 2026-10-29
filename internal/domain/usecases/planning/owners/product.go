package owners

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// A declaration read by more features than this says nothing about which belong
// together: a helper read by a hundred ties nothing, and counting it would chain
// every module to the next.
const kinReach = 12

func joinProducts(
	reachedBy map[entities.DeclKey]map[reach.FeatureID]bool,
	roots map[reach.FeatureID]bool,
) map[reach.FeatureID]reach.FeatureID {
	tie := map[reach.FeatureID]map[reach.FeatureID]float64{}

	// A shared declaration ties its readers with weight 1/(readers-1); a feature
	// joins the root it is tied to most, else stands alone.
	for _, features := range reachedBy {
		if len(features) < 2 || len(features) > kinReach {
			continue
		}

		weight := 1 / float64(len(features)-1)

		for feature := range features {
			for root := range features {
				if feature == root || !roots[root] {
					continue
				}

				if tie[feature] == nil {
					tie[feature] = map[reach.FeatureID]float64{}
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

func strongest(ties map[reach.FeatureID]float64) reach.FeatureID {
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
