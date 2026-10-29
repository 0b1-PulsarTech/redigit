package owners

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

type entryWalk struct {
	reachedBy    map[entities.DeclKey]map[reach.FeatureID]bool
	entryModules map[string]bool
	rootHeads    map[reach.FeatureID]bool
	rootOf       map[string]reach.FeatureID
	// built is the features some entry of production code starts. Code only
	// test support reaches names a feature that is never built.
	built map[reach.FeatureID]bool
	// order is the built features.
	order []reach.FeatureID
}

func walkEntries(graph entities.Graph, own Ownership, entries []entities.Decl) entryWalk {
	walk := entryWalk{
		reachedBy:    map[entities.DeclKey]map[reach.FeatureID]bool{},
		entryModules: map[string]bool{},
		rootHeads:    map[reach.FeatureID]bool{},
		rootOf:       map[string]reach.FeatureID{},
		built:        map[reach.FeatureID]bool{},
	}

	for _, entry := range entries {
		feature := reach.FeatureOf(entry)

		if !own.Support[entry.Key] && !walk.built[feature] {
			walk.built[feature] = true
			walk.order = append(walk.order, feature)
		}

		if isHead(graph, entry, own.Roots) {
			walk.entryModules[moduleDir(graph, entry)] = true

			if atModuleRoot(graph, entry) {
				walk.rootHeads[feature] = true
				walk.rootOf[moduleDir(graph, entry)] = feature
			}
		}

		for key := range reach.ReachFrom(graph, entry.Key) {
			if walk.reachedBy[key] == nil {
				walk.reachedBy[key] = map[reach.FeatureID]bool{}
			}

			walk.reachedBy[key][feature] = true
		}
	}

	return walk
}
