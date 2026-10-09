package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// isAfterword is a declaration that is only true once the code it names exists,
// such as a changelog entry about a route. It is no capability of its own: owned
// as an entry point it would form a feature of all the entries, born after every
// feature they name instead of after the one each names. It is deferred like a
// test and born right after the feature that completes what it waits for.
func isAfterword(graph entities.Graph, key entities.DeclKey) bool {
	for _, edge := range graph.Needs(key) {
		if edge.Reason == entities.ReasonProse {
			return true
		}
	}

	return false
}
