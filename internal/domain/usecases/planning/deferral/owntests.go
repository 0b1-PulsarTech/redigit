package deferral

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// OwnTests is the tests, outside the wiring, that the feature just built
// completes: everything they exercise is born, and the feature supplied part of
// it. One that needs a ready root waits for that root's turn.
func OwnTests(
	graph entities.Graph,
	own owners.Ownership,
	born, fresh map[entities.DeclKey]bool,
) []entities.DeclKey {
	subjects := reach.TestSubjects(graph)
	taken := map[entities.DeclKey]bool{}

	for _, key := range ReadyDeferred(graph, own, born, fresh) {
		if !own.Roots[key] && !born[key] {
			taken[key] = true
		}
	}

	for changed := true; changed; {
		changed = false

		for key := range taken {
			if !waitsOn(graph, subjects, key, func(provider entities.DeclKey) bool {
				return !born[provider] && !taken[provider] && own.Deferred[provider]
			}) {
				continue
			}

			delete(taken, key)

			changed = true
		}
	}

	return owners.SortedKeys(taken)
}
