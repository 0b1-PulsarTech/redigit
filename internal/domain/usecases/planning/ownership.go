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
	// loose is what nothing reads, deferred and born once what it names exists:
	// the manifest lines and the entries of a library.
	loose wiring
	// packages is, per unit, the shared declarations of production code that no
	// test and no deferral holds back.
	packages map[entities.UnitID][]entities.DeclKey
	// group is the product a feature belongs to, named by the module-root
	// feature it serves, so the features of one product are ordered together.
	group map[FeatureID]FeatureID
	// waits is the module roots a feature should come after.
	waits map[FeatureID]map[FeatureID]bool
	// cost is how many declarations a feature needs in all, its own and the
	// shared ones, so the smallest working feature can come first.
	cost map[FeatureID]int
}

// assign walks back from every entry point and counts, per declaration, how
// many distinct features can reach it. Counting insiders would pin every
// package to the instant the source history happened to create it.
func assign(graph entities.Graph) ownership {
	reachedBy := map[entities.DeclKey]map[FeatureID]bool{}
	seen := map[FeatureID]bool{}
	entryModules := map[string]bool{}
	rootHeads := map[FeatureID]bool{}
	rootOf := map[string]FeatureID{}

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{},
		reach: map[entities.DeclKey]int{},
		group: map[FeatureID]FeatureID{},
		cost:  map[FeatureID]int{},
		roots: codeWiring(graph, entities.FindWiring(graph)),
		loose: looseLines(graph),
	}

	own.deferred = deferTests(graph, own.roots)

	entries := entryPoints(graph, own.roots)

	for _, entry := range entries {
		feature := featureOf(entry)
		if !seen[feature] {
			seen[feature] = true

			own.order = append(own.order, feature)
		}

		if isHead(graph, entry, own.roots) {
			entryModules[moduleDir(graph, entry)] = true

			if atModuleRoot(graph, entry) {
				rootHeads[feature] = true
				rootOf[moduleDir(graph, entry)] = feature
			}
		}

		for key := range reachFrom(graph, entry.Key) {
			if reachedBy[key] == nil {
				reachedBy[key] = map[FeatureID]bool{}
			}

			reachedBy[key][feature] = true
		}
	}

	unionAtoms(graph, reachedBy)
	own.group = joinProducts(reachedBy, rootHeads)

	for key, features := range reachedBy {
		for feature := range features {
			own.cost[feature]++
		}

		own.reach[key] = len(features)
		own.owner[key] = soleOwner(features)
	}

	own.waits = readsRoots(graph, reachedBy, rootOf)

	shareUnreached(graph, own)

	shareLibraries(graph, own, entryModules)
	deferLibraryEntries(own, entries)

	// What is deferred is born when it is ready, not with a feature that
	// happens to own part of what it needs.
	for key := range own.deferred {
		own.owner[key] = SharedFeature
	}

	shareDependencies(graph, own)

	own.packages = sharedPackages(graph, own)

	slices.SortFunc(own.order, func(a, b FeatureID) int { return cmp.Compare(a, b) })

	return own
}

// shareUnreached shares what no entry point reaches: dead weight or a root
// nobody consumes yet. It is shared rather than silently dropped: dropping it
// would leave a hole in the final tree that the tree invariant catches far too
// late to explain.
func shareUnreached(graph entities.Graph, own ownership) {
	for _, decl := range graph.Decls() {
		if _, found := own.reach[decl.Key]; !found {
			own.reach[decl.Key] = 0
			own.owner[decl.Key] = SharedFeature
		}
	}
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
