package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/testplace"
)

// Ownership is which feature each declaration belongs to, and how many features
// reach it. The count is the whole decision: a declaration reached by one
// feature is part of that feature and is born with it; one reached by many is
// infrastructure and has to be born before all of them.
type Ownership struct {
	Owner map[entities.DeclKey]FeatureID
	Reach map[entities.DeclKey]int
	Order []FeatureID
	// Deferred is the wiring and its tests: owned only once the features are
	// in build order, by the last one they need.
	Deferred reach.Wiring
	Roots    reach.Wiring
	// Support is the code only other packages' tests read; it is deferred and
	// born with the first test that needs it.
	Support reach.Wiring
	// Loose is what nothing reads, deferred and born once what it names exists:
	// the manifest lines and the entries of a library.
	Loose reach.Wiring
	// packages is, per unit, the shared declarations of production code that no
	// test and no deferral holds back.
	packages map[entities.UnitID][]entities.DeclKey
	// Group is the product a feature belongs to, named by the module-root
	// feature it serves, so the features of one product are ordered together.
	Group map[FeatureID]FeatureID
	// Waits is the module roots a feature should come after.
	Waits map[FeatureID]map[FeatureID]bool
	// Cost is how many declarations a feature needs in all, its own and the
	// shared ones, so the smallest working feature can come first.
	Cost map[FeatureID]int
}

// Assign walks back from every entry point and counts, per declaration, how
// many distinct features can reach it. Counting insiders would pin every
// package to the instant the source history happened to create it.
func Assign(graph entities.Graph) Ownership {
	own := Ownership{
		Owner: map[entities.DeclKey]FeatureID{},
		Reach: map[entities.DeclKey]int{},
		Group: map[FeatureID]FeatureID{},
		Cost:  map[FeatureID]int{},
		Roots: reach.CodeWiring(graph, entities.FindWiring(graph)),
		Loose: reach.LooseLines(graph),
	}

	own.Support = testplace.TestSupport(graph)
	own.Deferred = reach.DeferTests(graph, own.Roots, own.Support)

	entries := reach.EntryPoints(graph, own.Roots)
	walk := walkEntries(graph, own, entries)

	unionAtoms(graph, walk.reachedBy)
	own.Group = joinProducts(walk.reachedBy, walk.rootHeads)
	own.Order = walk.order

	for key, features := range walk.reachedBy {
		for feature := range features {
			own.Cost[feature]++

			if walk.built[feature] {
				own.Reach[key]++
			}
		}

		own.Owner[key] = soleOwner(features)

		// The code only test support reaches has no feature to be built in.
		if !walk.built[own.Owner[key]] {
			own.Owner[key] = reach.SharedFeature
		}
	}

	own.Waits = readsRoots(graph, own.Owner, walk.rootOf)

	shareUnreached(graph, own)

	shareLibraries(graph, own, walk.entryModules)
	deferLibraryEntries(own, entries)

	// What is deferred is born when it is ready, not with a feature that
	// happens to own part of what it needs.
	for key := range own.Deferred {
		own.Owner[key] = reach.SharedFeature
	}

	shareCompanions(graph, own)

	shareDependencies(graph, own)

	own.packages = sharedPackages(graph, own)

	slices.SortFunc(own.Order, func(a, b FeatureID) int { return cmp.Compare(a, b) })

	return own
}

// shareUnreached shares what no entry point reaches: dead weight or a root
// nobody consumes yet. It is shared rather than silently dropped: dropping it
// would leave a hole in the final tree that the tree invariant catches far too
// late to explain.
func shareUnreached(graph entities.Graph, own Ownership) {
	for _, decl := range graph.Decls() {
		if _, found := own.Reach[decl.Key]; !found {
			own.Reach[decl.Key] = 0
			own.Owner[decl.Key] = reach.SharedFeature
		}
	}
}

func unionAtoms(graph entities.Graph, reachedBy map[entities.DeclKey]map[FeatureID]bool) {
	for _, members := range reach.AtomGroups(graph) {
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
		return reach.SharedFeature
	}

	for feature := range features {
		return feature
	}

	return reach.SharedFeature
}
