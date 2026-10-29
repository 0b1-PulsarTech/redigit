package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// IsAfterword is a declaration that is only true once the code it names exists,
// such as a changelog entry about a route. It is no capability of its own: owned
// as an entry point it would form a feature of all the entries, born after every
// feature they name instead of after the one each names. It is deferred like a
// test and born right after the feature that completes what it waits for.
func IsAfterword(graph entities.Graph, key entities.DeclKey) bool {
	for _, edge := range graph.Needs(key) {
		if edge.Reason == entities.ReasonProse {
			return true
		}
	}

	return false
}

// IsAfterwordBase is what only afterwords build on and that needs nothing
// itself, such as a changelog's title: it is deferred with them and born with the
// first of them, since no feature leads to it and it must not trail them.
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

// CodeWiring is the wiring that is code. A document naming many directories
// reaches a twentieth of the program and looked like a root, which made every
// entry it names a head plugged into it and tied unrelated modules to a product.
func CodeWiring(graph entities.Graph, roots Wiring) Wiring {
	out := Wiring{}

	for key := range roots {
		if !IsAfterword(graph, key) && !IsAfterwordBase(graph, key) {
			out[key] = true
		}
	}

	return out
}
