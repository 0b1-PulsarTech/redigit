package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// ownership is which feature each declaration belongs to, and how many features
// reach it. The count is the whole decision: a declaration reached by one
// feature is part of that feature and is born with it; one reached by many is
// infrastructure and has to be born before all of them.
type ownership struct {
	owner map[entities.DeclKey]FeatureID
	reach map[entities.DeclKey]int
	order []FeatureID
	// deferred is the wiring and its tests: owned only once the features are
	// in build order, by the last one they need.
	deferred wiring
	roots    wiring
	// group is the module a feature's entry point is built in, so the features
	// of one module are ordered together.
	group map[FeatureID]string
	// cost is how many declarations a feature needs in all, its own and the
	// shared ones, so the smallest working feature can come first.
	cost map[FeatureID]int
	// library maps a declaration of a module with no entry point to that
	// module, and libraries lists the module's production declarations.
	library   map[entities.DeclKey]string
	libraries map[string][]entities.DeclKey
}

// assign walks back from every entry point and counts, per declaration, how
// many distinct features can reach it.
//
// This is the same shape as the rule that a unit is born when someone OUTSIDE
// it needs it, counting features instead of units. Getting that count wrong by
// including insiders is what pinned every package to whatever instant the
// source history happened to create it.
func assign(graph entities.Graph) ownership {
	reachedBy := map[entities.DeclKey]map[FeatureID]bool{}
	seen := map[FeatureID]bool{}
	entryModules := map[string]bool{}

	own := ownership{
		owner:     map[entities.DeclKey]FeatureID{},
		reach:     map[entities.DeclKey]int{},
		group:     map[FeatureID]string{},
		cost:      map[FeatureID]int{},
		library:   map[entities.DeclKey]string{},
		libraries: map[string][]entities.DeclKey{},
	}

	roots := findWiring(graph)
	own.roots = roots
	own.deferred = deferTests(graph, roots)

	for _, entry := range entryPoints(graph, roots) {
		feature := featureOf(entry)
		if !seen[feature] {
			seen[feature] = true

			own.order = append(own.order, feature)
			own.group[feature] = moduleDir(graph, entry)
		}

		if isHead(graph, entry, roots) {
			entryModules[moduleDir(graph, entry)] = true
		}

		for key := range reachFrom(graph, entry.Key) {
			if reachedBy[key] == nil {
				reachedBy[key] = map[FeatureID]bool{}
			}

			reachedBy[key][feature] = true
		}
	}

	unionAtoms(graph, reachedBy)

	for key, features := range reachedBy {
		for feature := range features {
			own.cost[feature]++
		}

		own.reach[key] = len(features)
		own.owner[key] = soleOwner(features)
	}

	// A declaration no entry point reaches is dead weight or a root nobody
	// consumes yet. It is shared rather than silently dropped: dropping it
	// would leave a hole in the final tree that the tree invariant catches
	// far too late to explain.
	for _, decl := range graph.Decls() {
		if _, found := own.reach[decl.Key]; !found {
			own.reach[decl.Key] = 0
			own.owner[decl.Key] = SharedFeature
		}
	}

	shareLibraries(graph, own, entryModules)

	// What is deferred is born when it is ready, not with a feature that
	// happens to own part of what it needs.
	for key := range own.deferred {
		own.owner[key] = SharedFeature
	}

	shareDependencies(graph, own)

	slices.SortFunc(own.order, func(a, b FeatureID) int { return cmp.Compare(a, b) })

	return own
}

func unionAtoms(graph entities.Graph, reachedBy map[entities.DeclKey]map[FeatureID]bool) {
	for _, members := range atomGroups(graph) {
		union := map[FeatureID]bool{}

		for _, key := range members {
			for feature := range reachedBy[key] {
				union[feature] = true
			}
		}

		if len(union) == 0 {
			continue
		}

		for _, key := range members {
			reachedBy[key] = union
		}
	}
}

// soleOwner is deliberately not a threshold. A declaration reached by two
// features belongs to neither: picking one of them would be a guess, and a
// guess here is what puts a declaration hundreds of commits from its use.
func soleOwner(features map[FeatureID]bool) FeatureID {
	if len(features) != 1 {
		return SharedFeature
	}

	for feature := range features {
		return feature
	}

	return SharedFeature
}
