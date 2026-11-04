package ordering

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func splitAtHosts(graph entities.Graph, cycle entities.Cycle) []entities.Cycle {
	if !slices.Contains(cycle.Reasons, entities.ReasonHosts) {
		return []entities.Cycle{cycle}
	}

	members := make(map[entities.DeclKey]bool, len(cycle.Members))
	for _, member := range cycle.Members {
		members[member] = true
	}

	// Without its hosts edges onto code a cycle is a helper born with its main;
	// only the recursion left is still a cycle. A manifest is not code: the
	// edge onto it stays, so a manifest that reads its script is still reported.
	keep := func(edge entities.Edge) bool {
		isCoBirth := edge.Reason == entities.ReasonHosts && edge.Provider.Kind.IsCode()

		return !isCoBirth && members[edge.Provider]
	}

	var parts []entities.Cycle

	for _, group := range components(graph, cycle.Members, keep) {
		part := buildCycle(graph, group)
		if part.SpansFiles() {
			return []entities.Cycle{cycle}
		}

		parts = append(parts, part)
	}

	return parts
}
