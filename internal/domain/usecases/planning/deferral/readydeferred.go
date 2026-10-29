package deferral

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func ReadyDeferred(
	graph entities.Graph,
	own owners.Ownership,
	born map[entities.DeclKey]bool,
	fresh map[entities.DeclKey]bool,
) []entities.DeclKey {
	subjects := sharedSubjects(graph, own)
	atoms := reach.AtomGroups(graph)
	ready := map[entities.DeclKey]bool{}

	for key := range own.Deferred {
		if !born[key] {
			ready[key] = true
		}
	}

	// A test is born once everything it exercises is: pulling what it needs
	// would put the code under it in the feature that happened to come first.
	waits := func(provider entities.DeclKey, strict bool) bool {
		switch {
		case born[provider]:
			return false
		case own.Deferred[provider]:
			return !ready[provider]
		case own.Owner[provider] != reach.SharedFeature:
			return true
		case own.Reach[provider] == 0:
			return false
		default:
			return strict && !isManifestKey(graph, provider) ||
				usedBeyondWiring(graph, own, provider)
		}
	}

	for changed := true; changed; {
		changed = false

		for key := range ready {
			strict := !own.Roots[key]

			if !waitsOn(graph, subjects, key, func(provider entities.DeclKey) bool {
				return waits(provider, strict)
			}) && !atomWaits(atomMembers(graph, atoms, key), own, born, ready) {
				continue
			}

			delete(ready, key)

			changed = true
		}
	}

	// Only what needs something fresh is ready: otherwise whatever needs only
	// shared code is ready at the first feature and drags that code in.
	return owners.SortedKeys(anchorHelpers(graph, own, touched(graph, subjects, own, ready, fresh), ready))
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

func atomWaits(
	members []entities.DeclKey,
	own owners.Ownership,
	born, ready map[entities.DeclKey]bool,
) bool {
	for _, member := range members {
		if own.Deferred[member] && !born[member] && !ready[member] {
			return true
		}
	}

	return false
}

func usedBeyondWiring(graph entities.Graph, own owners.Ownership, key entities.DeclKey) bool {
	for _, edge := range graph.Consumers(key) {
		consumer, found := graph.Decl(edge.Dependent)
		if !found || entities.IsTestSide(consumer) || entities.IsStructural(edge.Reason) {
			continue
		}

		if !own.Roots[edge.Dependent] && !own.Deferred[edge.Dependent] {
			return true
		}
	}

	return false
}

func isManifestKey(graph entities.Graph, key entities.DeclKey) bool {
	decl, found := graph.Decl(key)

	return found && reach.IsManifestLine(graph, decl)
}
