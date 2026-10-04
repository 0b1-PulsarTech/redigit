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

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{},
		reach: map[entities.DeclKey]int{},
	}

	for _, entry := range entryPoints(graph) {
		feature := featureOf(entry)
		if !seen[feature] {
			seen[feature] = true

			own.order = append(own.order, feature)
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

	inheritTestOwnership(graph, own)
	inheritTestFileOwnership(graph, own)
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
