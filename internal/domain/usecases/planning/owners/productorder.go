package owners

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// readsRoots is, per feature, the module roots whose code it reads directly: the
// roots it should wait for, so a module does not open as a stray file inside
// another arc. What it reaches only through shared code is not read by it, and
// waiting for that would make every feature wait for the whole program. The
// wait is a preference: modules that read each other are built anyway.
func readsRoots(
	graph entities.Graph,
	owner map[entities.DeclKey]reach.FeatureID,
	rootOf map[string]reach.FeatureID,
) map[reach.FeatureID]map[reach.FeatureID]bool {
	reads := map[reach.FeatureID]map[reach.FeatureID]bool{}

	for _, edge := range graph.Edges() {
		feature := owner[edge.Dependent]
		if feature == reach.SharedFeature || entities.IsStructural(edge.Reason) ||
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
			reads[feature] = map[reach.FeatureID]bool{}
		}

		reads[feature][root] = true
	}

	return reads
}

// ProductOf is the root of the product a feature serves; a feature serving none
// is its own.
func ProductOf(group map[reach.FeatureID]reach.FeatureID, feature reach.FeatureID) reach.FeatureID {
	if root, found := group[feature]; found {
		return root
	}

	return feature
}
