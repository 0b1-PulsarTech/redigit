package planning

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// ownTests is the tests, outside the wiring, that the feature just built
// completes: everything they exercise is born, and the feature supplied part of
// it. One that needs a ready root waits for that root's turn.
func ownTests(
	graph entities.Graph,
	own ownership,
	born, fresh map[entities.DeclKey]bool,
) []entities.DeclKey {
	subjects := reach.TestSubjects(graph)
	taken := map[entities.DeclKey]bool{}

	for _, key := range readyDeferred(graph, own, born, fresh) {
		if !own.roots[key] && !born[key] {
			taken[key] = true
		}
	}

	for changed := true; changed; {
		changed = false

		for key := range taken {
			if !waitsOn(graph, subjects, key, func(provider entities.DeclKey) bool {
				return !born[provider] && !taken[provider] && own.deferred[provider]
			}) {
				continue
			}

			delete(taken, key)

			changed = true
		}
	}

	return sortedKeys(taken)
}
