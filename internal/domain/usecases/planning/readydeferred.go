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
	subjects := testSubjects(graph)
	atoms := atomGroups(graph)
	ready := map[entities.DeclKey]bool{}

	for key := range own.deferred {
		if !born[key] {
			ready[key] = true
		}
	}

	// A test is born once everything it exercises is: pulling what it needs
	// would put the code under it in the feature that happened to come first.
	// The wiring has no feature of its own, so it may bring what only it uses.
	waits := func(provider entities.DeclKey, strict bool) bool {
		switch {
		case born[provider]:
			return false
		case own.deferred[provider]:
			return !ready[provider]
		case own.owner[provider] != SharedFeature:
			return true
		default:
			return strict
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

	return sortedKeys(touched(graph, subjects, ready, fresh))
}

// touched keeps the ready declarations that need something fresh, directly or
// through another ready declaration. A manifest line does not count: every test
// of a module needs its go.mod, and that is no reason for the test to arrive
// with the first declaration that pulled the manifest in.
func touched(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	ready, fresh map[entities.DeclKey]bool,
) map[entities.DeclKey]bool {
	out := map[entities.DeclKey]bool{}

	for changed := true; changed; {
		changed = false

		for key := range ready {
			if out[key] || !needsFresh(graph, subjects, key, fresh, out) {
				continue
			}

			out[key] = true
			changed = true
		}
	}

	return out
}

func needsFresh(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	key entities.DeclKey,
	fresh, touched map[entities.DeclKey]bool,
) bool {
	for _, edge := range graph.Needs(key) {
		if entities.IsStructural(edge.Reason) || edge.Reason == entities.ReasonImport {
			continue
		}

		if fresh[edge.Provider] || touched[edge.Provider] {
			return true
		}
	}

	for _, subject := range subjects[key] {
		if fresh[subject] || touched[subject] {
			return true
		}
	}

	return false
}

func waitsOn(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	key entities.DeclKey,
	waits func(entities.DeclKey) bool,
) bool {
	for _, edge := range graph.Needs(key) {
		if waits(edge.Provider) {
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
