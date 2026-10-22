package planning

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// readyDeferred is the deferred declarations that can be born now: none of what
// they need belongs to a feature still to come, and something they need was
// born with the feature just built. Without the second condition, whatever
// needs only shared code is ready at the first feature and drags that code in.
func readyDeferred(
	graph entities.Graph,
	own ownership,
	born map[entities.DeclKey]bool,
	fresh map[entities.DeclKey]bool,
) []entities.DeclKey {
	subjects := sharedSubjects(graph, own)
	atoms := atomGroups(graph)
	ready := map[entities.DeclKey]bool{}

	for key := range own.deferred {
		if !born[key] {
			ready[key] = true
		}
	}

	// A test is born once everything it exercises is: pulling what it needs
	// would put the code under it in the feature that happened to come first.
	// The wiring has no feature of its own, so it may bring what only it uses;
	// what a feature also uses is born with that feature, and bringing it early
	// would pull its whole closure into the first registration. A manifest line
	// is brought by what imports it: nothing else would ever birth the require
	// of a module that only tests import, and so is what no built feature
	// reaches.
	waits := func(provider entities.DeclKey, strict bool) bool {
		switch {
		case born[provider]:
			return false
		case own.deferred[provider]:
			return !ready[provider]
		case own.owner[provider] != SharedFeature:
			return true
		case own.reach[provider] == 0:
			return false
		default:
			return strict && !isManifestKey(graph, provider) ||
				usedBeyondWiring(graph, own, provider)
		}
	}

	for changed := true; changed; {
		changed = false

		for key := range ready {
			strict := !own.roots[key]

			if !waitsOn(graph, subjects, key, func(provider entities.DeclKey) bool {
				return waits(provider, strict)
			}) && !atomWaits(atomMembers(graph, atoms, key), own, born, ready) {
				continue
			}

			delete(ready, key)

			changed = true
		}
	}

	return sortedKeys(anchorHelpers(graph, own, touched(graph, subjects, own, ready, fresh), ready))
}

func waitsOn(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	key entities.DeclKey,
	waits func(entities.DeclKey) bool,
) bool {
	for _, edge := range graph.Needs(key) {
		if !entities.FillsInterface(graph, edge) && waits(edge.Provider) {
			return true
		}
	}

	return slices.ContainsFunc(subjects[key], waits)
}

// atomMembers is the members of a declaration's atom, none when it has no atom.
func atomMembers(
	graph entities.Graph,
	atoms map[string][]entities.DeclKey,
	key entities.DeclKey,
) []entities.DeclKey {
	decl, found := graph.Decl(key)
	if !found || decl.Atom == "" {
		return nil
	}

	return atoms[decl.Atom]
}

// atomWaits is whether an atom has a deferred member that is not ready: the
// members are born in one commit, so the first to be ready would otherwise pull
// the others in with everything they need.
func atomWaits(
	members []entities.DeclKey,
	own ownership,
	born, ready map[entities.DeclKey]bool,
) bool {
	for _, member := range members {
		if own.deferred[member] && !born[member] && !ready[member] {
			return true
		}
	}

	return false
}

// usedBeyondWiring is whether a shared declaration has a consumer that is not
// itself wiring or a test, a consumer some feature will be built for.
func usedBeyondWiring(graph entities.Graph, own ownership, key entities.DeclKey) bool {
	for _, edge := range graph.Consumers(key) {
		consumer, found := graph.Decl(edge.Dependent)
		if !found || entities.IsTestSide(consumer) || entities.IsStructural(edge.Reason) {
			continue
		}

		if !own.roots[edge.Dependent] && !own.deferred[edge.Dependent] {
			return true
		}
	}

	return false
}

func isManifestKey(graph entities.Graph, key entities.DeclKey) bool {
	decl, found := graph.Decl(key)

	return found && isManifestLine(graph, decl)
}
