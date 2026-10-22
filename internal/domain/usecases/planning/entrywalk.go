package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// entryWalk is what walking back from every entry point finds.
type entryWalk struct {
	reachedBy    map[entities.DeclKey]map[FeatureID]bool
	entryModules map[string]bool
	rootHeads    map[FeatureID]bool
	rootOf       map[string]FeatureID
	// built is the features some entry of production code starts. Code only
	// test support reaches names a feature that is never built.
	built map[FeatureID]bool
	// order is the built features.
	order []FeatureID
}

func walkEntries(graph entities.Graph, own ownership, entries []entities.Decl) entryWalk {
	walk := entryWalk{
		reachedBy:    map[entities.DeclKey]map[FeatureID]bool{},
		entryModules: map[string]bool{},
		rootHeads:    map[FeatureID]bool{},
		rootOf:       map[string]FeatureID{},
		built:        map[FeatureID]bool{},
	}

	for _, entry := range entries {
		feature := featureOf(entry)

		if !own.support[entry.Key] && !walk.built[feature] {
			walk.built[feature] = true
			walk.order = append(walk.order, feature)
		}

		if isHead(graph, entry, own.roots) {
			walk.entryModules[moduleDir(graph, entry)] = true

			if atModuleRoot(graph, entry) {
				walk.rootHeads[feature] = true
				walk.rootOf[moduleDir(graph, entry)] = feature
			}
		}

		for key := range reachFrom(graph, entry.Key) {
			if walk.reachedBy[key] == nil {
				walk.reachedBy[key] = map[FeatureID]bool{}
			}

			walk.reachedBy[key][feature] = true
		}
	}

	return walk
}
