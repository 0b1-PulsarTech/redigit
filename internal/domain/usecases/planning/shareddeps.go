package planning

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// shareDependencies makes the shared bucket closed under what it needs: shared
// code is born before every feature, so a provider owned by one feature would
// be born after its reader. Atoms, tests and test helpers escape the reach
// count, so a provider of a shared declaration is shared too, and an atom goes
// whole.
func shareDependencies(graph entities.Graph, own Ownership) {
	atoms := reach.AtomGroups(graph)
	queue := make([]entities.DeclKey, 0, graph.Len())

	for _, decl := range graph.Decls() {
		if own.Owner[decl.Key] == reach.SharedFeature && !own.Deferred[decl.Key] {
			queue = append(queue, decl.Key)
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		provided := []entities.DeclKey{}

		for _, edge := range graph.Needs(current) {
			provided = append(provided, edge.Provider)
		}

		if decl, found := graph.Decl(current); found && decl.Atom != "" {
			provided = append(provided, atoms[decl.Atom]...)
		}

		for _, key := range provided {
			owner, known := own.Owner[key]
			if !known || owner == reach.SharedFeature {
				continue
			}

			own.Owner[key] = reach.SharedFeature
			own.Reach[key] = max(own.Reach[key], own.Reach[current])

			queue = append(queue, key)
		}
	}
}

// takesForeign is whether a declaration walked into the shared commit brings
// the providers a feature owns. A statement of the assembly is deferred, so
// assign never shared what it reads; walked in ahead of its feature, it would
// be born before them, and so would whatever those read in turn.
func (index sharedIndex) takesForeign(own Ownership, current entities.DeclKey) bool {
	if own.Roots[current] {
		return true
	}

	return index.walked[current] && own.Owner[current] != reach.SharedFeature
}
