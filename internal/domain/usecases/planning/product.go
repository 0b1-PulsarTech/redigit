package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// kinReach is the most features that may read a declaration for it to say
// anything about which of them belong together. A port read by a module's route
// group, repository and worker ties the three; a helper read by a hundred
// features ties nothing, and counting it would chain every module to the next.
const kinReach = 12

// joinProducts groups each feature with the module root it serves.
//
// A product is spread over the tiers: its route group, its repository and
// its worker are built in three different modules, and no directory or manifest
// names them as one. What they have in common is the contract they all read, a
// port or an entity that only they read, and the module root that wires the
// product into the program reads it too. Grouping features by the module
// they are built in instead put every route group together, then every
// repository together, and a product module's arc spanned the whole history.
//
// A shared declaration ties its readers with weight 1/(readers-1), so the tie
// falls as the declaration becomes common. A feature joins the root it is
// tied to most; one tied to none, or a root itself, stands alone.
func joinProducts(
	reachedBy map[entities.DeclKey]map[FeatureID]bool,
	roots map[FeatureID]bool,
) map[FeatureID]FeatureID {
	tie := map[FeatureID]map[FeatureID]float64{}

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
					tie[feature] = map[FeatureID]float64{}
				}

				tie[feature][root] += weight
			}
		}
	}

	out := map[FeatureID]FeatureID{}

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
func strongest(ties map[FeatureID]float64) FeatureID {
	candidates := make([]FeatureID, 0, len(ties))
	for root := range ties {
		candidates = append(candidates, root)
	}

	return slices.MaxFunc(candidates, func(a, b FeatureID) int {
		if byTie := cmp.Compare(ties[a], ties[b]); byTie != 0 {
			return byTie
		}

		return cmp.Compare(b, a)
	})
}
