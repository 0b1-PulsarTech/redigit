package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func edgeOf(dependent, provider entities.Decl, reason entities.EdgeReason) entities.Edge {
	return entities.Edge{Dependent: dependent.Key, Provider: provider.Key, Reason: reason}
}

func TestHasProductionConsumerIgnoresAnAfterword(t *testing.T) {
	t.Parallel()

	route := decl("a/handler.go", "Group", "a")
	entry := decl("docs/CHANGELOG.md", "<file>#2", "docs")
	graph := entities.NewGraph(
		[]entities.Decl{route, entry},
		[]entities.Edge{edgeOf(entry, route, entities.ReasonProse)},
		nil,
	)

	if hasProductionConsumer(graph, route.Key, wiring{}) {
		t.Fatal("a changelog line counted as a production consumer of the route it describes")
	}
}
