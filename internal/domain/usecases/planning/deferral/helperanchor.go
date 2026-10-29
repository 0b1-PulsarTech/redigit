package deferral

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
)

func anchorHelpers(
	graph entities.Graph,
	own owners.Ownership,
	chosen, pool map[entities.DeclKey]bool,
) map[entities.DeclKey]bool {
	kept := map[entities.DeclKey]bool{}
	queue := []entities.DeclKey{}

	for key := range chosen {
		if !isHelper(graph, own, key) {
			kept[key] = true

			queue = append(queue, key)
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range graph.Needs(current) {
			provider := edge.Provider
			if kept[provider] || !pool[provider] || !isHelper(graph, own, provider) {
				continue
			}

			kept[provider] = true

			queue = append(queue, provider)
		}
	}

	return kept
}

func isHelper(graph entities.Graph, own owners.Ownership, key entities.DeclKey) bool {
	decl, found := graph.Decl(key)
	// A non-test declaration nothing else reads is not a helper, only a loose one.
	if !found || decl.IsTest || !entities.IsTestSide(decl) && !own.Support[key] {
		return false
	}

	for _, edge := range graph.Consumers(key) {
		if edge.Dependent != key && !entities.IsStructural(edge.Reason) {
			return true
		}
	}

	return false
}
