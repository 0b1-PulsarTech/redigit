package reach

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func IsAfterword(graph entities.Graph, key entities.DeclKey) bool {
	for _, edge := range graph.Needs(key) {
		if edge.Reason == entities.ReasonProse {
			// An afterword is no capability of its own: owned as an entry point it
			// would form a feature of all the entries, born after every feature they
			// name instead of after the one each names.
			return true
		}
	}

	return false
}

func IsAfterwordBase(graph entities.Graph, key entities.DeclKey) bool {
	consumers := graph.Consumers(key)
	if key.Kind != entities.KindOther || len(consumers) == 0 || len(graph.Needs(key)) > 0 {
		return false
	}

	for _, edge := range consumers {
		if edge.Reason != entities.ReasonProse || !IsAfterword(graph, edge.Dependent) {
			return false
		}
	}

	return true
}

func CodeWiring(graph entities.Graph, roots Wiring) Wiring {
	out := Wiring{}

	for key := range roots {
		if !IsAfterword(graph, key) && !IsAfterwordBase(graph, key) {
			out[key] = true
		}
	}

	return out
}
