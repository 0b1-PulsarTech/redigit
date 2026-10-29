package owners

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/birth"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func (index sharedIndex) safeToPull(
	graph entities.Graph,
	key entities.DeclKey,
	born map[entities.DeclKey]bool,
) bool {
	seen := map[entities.DeclKey]bool{key: true}
	// A shared test is pulled along with its reader, so it is checked as the
	// reader is: its own needs can belong to a feature that is not built yet.
	queue := []entities.DeclKey{key}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// A subject the shared commit does not carry is born after it, so the test
		// would land ahead of the code it exercises.
		for _, subject := range index.subjects[current] {
			if !born[subject] && !index.walked[subject] {
				return false
			}
		}

		for _, edge := range graph.Needs(current) {
			provider := edge.Provider

			switch {
			case birth.IsSoft(edge.Reason) || seen[provider] || born[provider]:
				continue
			case index.own.Owner[provider] != reach.SharedFeature:
				return false
			}

			seen[provider] = true

			queue = append(queue, provider)
		}

		if decl, found := graph.Decl(current); found && decl.Atom != "" {
			for _, member := range index.atoms[decl.Atom] {
				if !seen[member] {
					seen[member] = true

					queue = append(queue, member)
				}
			}
		}
	}

	return true
}

func driftsToTest(graph entities.Graph, edge entities.Edge) bool {
	provider, found := graph.Decl(edge.Provider)

	// The mock is read by that very test, so following the edge would carry the
	// test, and what it needs, ahead of the mock.
	return birth.IsSoft(edge.Reason) && found && entities.IsTestSide(provider)
}
