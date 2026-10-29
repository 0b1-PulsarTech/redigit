package owners

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/testplace"
)

type Ownership struct {
	Owner map[entities.DeclKey]reach.FeatureID
	Reach map[entities.DeclKey]int
	Order []reach.FeatureID
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
	Group map[reach.FeatureID]reach.FeatureID
	// Waits is the module roots a feature should come after.
	Waits map[reach.FeatureID]map[reach.FeatureID]bool
	// Cost is how many declarations a feature needs in all, its own and the
	// shared ones, so the smallest working feature can come first.
	Cost map[reach.FeatureID]int
}

func Assign(graph entities.Graph) Ownership {
	own := Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{},
		Reach: map[entities.DeclKey]int{},
		Group: map[reach.FeatureID]reach.FeatureID{},
		Cost:  map[reach.FeatureID]int{},
		Roots: reach.CodeWiring(graph, entities.FindWiring(graph)),
		Loose: reach.LooseLines(graph),
	}

	own.Support = testplace.TestSupport(graph)
	own.Deferred = reach.DeferTests(graph, own.Roots, own.Support)

	entries := reach.EntryPoints(graph, own.Roots)
	// Each declaration is counted once per distinct feature that reaches it;
	// counting insiders would pin every package to the instant the source
	// history happened to create it.
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

	slices.SortFunc(
		own.Order,
		func(first, second reach.FeatureID) int { return cmp.Compare(first, second) },
	)

	return own
}

func shareUnreached(graph entities.Graph, own Ownership) {
	for _, decl := range graph.Decls() {
		if _, found := own.Reach[decl.Key]; !found {
			own.Reach[decl.Key] = 0
			own.Owner[decl.Key] = reach.SharedFeature
		}
	}
}

func unionAtoms(graph entities.Graph, reachedBy map[entities.DeclKey]map[reach.FeatureID]bool) {
	for _, members := range reach.AtomGroups(graph) {
		union := map[reach.FeatureID]bool{}

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

func soleOwner(features map[reach.FeatureID]bool) reach.FeatureID {
	if len(features) != 1 {
		// A declaration reached by two features belongs to neither: picking one of
		// them would be a guess, and a guess here is what puts a declaration
		// hundreds of commits from its use.
		return reach.SharedFeature
	}

	for feature := range features {
		return feature
	}

	return reach.SharedFeature
}
