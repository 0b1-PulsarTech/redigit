package owners

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// kinReach is the most features that may read a declaration for it to say
// anything about which of them belong together. A port read by a module's route
// group, repository and worker ties the three; a helper read by a hundred
// features ties nothing, and counting it would chain every module to the next.
const kinReach = 12

// joinProducts groups each feature with the module root it serves, through the
// contract they alone read. A shared declaration ties its readers with weight
// 1/(readers-1); a feature joins the root it is tied to most, else stands alone.
func joinProducts(
	reachedBy map[entities.DeclKey]map[reach.FeatureID]bool,
	roots map[reach.FeatureID]bool,
) map[reach.FeatureID]reach.FeatureID {
	tie := map[reach.FeatureID]map[reach.FeatureID]float64{}

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

// strongest is the root with the largest tie; the name settles an equal one so
// a plan is the same on every run.
func strongest(ties map[reach.FeatureID]float64) reach.FeatureID {
	candidates := make([]reach.FeatureID, 0, len(ties))
	for root := range ties {
		candidates = append(candidates, root)
	}

	return slices.MaxFunc(candidates, func(a, b reach.FeatureID) int {
		if byTie := cmp.Compare(ties[a], ties[b]); byTie != 0 {
			return byTie
		}

		return cmp.Compare(b, a)
	})
}
