package testplace

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestSupport(graph entities.Graph) reach.Wiring {
	out := reach.Wiring{}

	for changed := true; changed; {
		changed = false

		for _, decl := range graph.Decls() {
			if out[decl.Key] || decl.Key.Kind == entities.KindOther || entities.IsTestSide(decl) {
				continue
			}

			if !onlyOtherTestsRead(graph, decl, out) {
				continue
			}

			out[decl.Key] = true
			changed = true
		}
	}

	return out
}

func onlyOtherTestsRead(graph entities.Graph, decl entities.Decl, support reach.Wiring) bool {
	read := false

	for _, edge := range graph.Consumers(decl.Key) {
		consumer, found := graph.Decl(edge.Dependent)
		if !found || entities.IsStructural(edge.Reason) ||
			reach.IsAfterword(graph, edge.Dependent) {
			continue
		}

		if !entities.IsTestSide(consumer) && !support[edge.Dependent] {
			return false
		}

		// A function only its own package's tests read is the code under test,
		// not support.
		read = read || support[edge.Dependent] || consumer.Unit != decl.Unit
	}

	return read
}
