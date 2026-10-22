package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// readsRoots is, per feature, the module roots whose code it reads directly: the
// roots it should wait for, so a module does not open as a stray file inside
// another arc. What it reaches only through shared code is not read by it, and
// waiting for that would make every feature wait for the whole program. The
// wait is a preference: modules that read each other are built anyway.
func readsRoots(
	graph entities.Graph,
	owner map[entities.DeclKey]FeatureID,
	rootOf map[string]FeatureID,
) map[FeatureID]map[FeatureID]bool {
	reads := map[FeatureID]map[FeatureID]bool{}

	for _, edge := range graph.Edges() {
		feature := owner[edge.Dependent]
		if feature == SharedFeature || entities.IsStructural(edge.Reason) ||
			entities.FillsInterface(graph, edge) {
			continue
		}

		decl, found := graph.Decl(edge.Provider)
		if !found || decl.Key.Kind == entities.KindOther {
			continue
		}

		root, rooted := rootOf[moduleDir(graph, decl)]
		if !rooted || feature == root {
			continue
		}

		if reads[feature] == nil {
			reads[feature] = map[FeatureID]bool{}
		}

		reads[feature][root] = true
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
