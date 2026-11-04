package deferral

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func OwnTests(
	graph entities.Graph,
	own owners.Ownership,
	born, fresh map[entities.DeclKey]bool,
) []entities.DeclKey {
	subjects := reach.TestSubjects(graph)
	taken := map[entities.DeclKey]bool{}
	atoms := reach.AtomGroups(graph)

	for _, key := range readyWithin(graph, own, atoms, born, fresh) {
		if !own.Roots[key] && !born[key] {
			taken[key] = true
		}
	}

	// An atom is born in one commit: a member held back holds its mates back.
	held := func(mate entities.DeclKey) bool {
		return !born[mate] && !taken[mate]
	}

	for changed := true; changed; {
		changed = false
		memo := map[string]bool{}

		for key := range taken {
			if !waitsOn(graph, subjects, key, func(provider entities.DeclKey) bool {
				return !born[provider] && !taken[provider] && own.Deferred[provider]
			}) && !atomBlocked(graph, atoms, key, memo, held) {
				continue
			}

			delete(taken, key)

			changed = true
		}
	}

	return owners.SortedKeys(taken)
}
