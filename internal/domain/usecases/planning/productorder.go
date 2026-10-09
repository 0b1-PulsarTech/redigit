package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// readsRoots is, per feature, the module roots whose code it reads: the roots it
// should wait for.
//
// What a feature reads of another module's code is born with whichever feature
// reads it first. When that feature is built before the module's own root, the
// module opens as a stray file inside someone else's arc, long before the rest
// of it arrives. The wait is a preference, not a need: two modules that read
// each other leave nothing to wait for, and the plan builds them anyway.
func readsRoots(
	graph entities.Graph,
	reachedBy map[entities.DeclKey]map[FeatureID]bool,
	rootOf map[string]FeatureID,
) map[FeatureID]map[FeatureID]bool {
	reads := map[FeatureID]map[FeatureID]bool{}

	for key, features := range reachedBy {
		decl, found := graph.Decl(key)
		if !found || decl.Key.Kind == entities.KindOther {
			continue
		}

		root, rooted := rootOf[moduleDir(graph, decl)]
		if !rooted {
			continue
		}

		for feature := range features {
			if feature == root {
				continue
			}

			if reads[feature] == nil {
				reads[feature] = map[FeatureID]bool{}
			}

			reads[feature][root] = true
		}
	}

	return reads
}

// productOf is the root of the product a feature serves; a feature serving none
// is its own.
func productOf(group map[FeatureID]FeatureID, feature FeatureID) FeatureID {
	if root, found := group[feature]; found {
		return root
	}

	return feature
}
