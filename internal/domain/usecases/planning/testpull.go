package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// safeToPull is whether a test can be born with the shared kernel: everything it
// needs, and everything the shared tests it needs themselves need, is born or
// shared. A shared test is pulled along with its reader, so it is checked as
// the reader is: its own needs can belong to a feature that is not built yet.
func (index sharedIndex) safeToPull(
	graph entities.Graph,
	key entities.DeclKey,
	born map[entities.DeclKey]bool,
) bool {
	seen := map[entities.DeclKey]bool{key: true}
	queue := []entities.DeclKey{key}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range graph.Needs(current) {
			provider := edge.Provider

			switch {
			case isSoft(edge.Reason) || seen[provider] || born[provider]:
				continue
			case index.own.owner[provider] != SharedFeature:
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

// driftsToTest is a soft edge that points at a test: a mock generated from a
// directive that sits in a test file. The mock is read by that very test, so
// following the edge would carry the test, and what it needs, ahead of the mock.
func driftsToTest(graph entities.Graph, edge entities.Edge) bool {
	provider, found := graph.Decl(edge.Provider)

	return isSoft(edge.Reason) && found && isTestSide(provider)
}
