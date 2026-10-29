package deferral

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestReadyDeferredIgnoresAnInterfaceATypeOnlyFills(t *testing.T) {
	t.Parallel()

	subject := decl("a/create.go", "Create", "a")
	port := decl("b/port.go", "Port", "b")
	port.IsInterface = true
	double := testDecl("a/create_test.go", "stub", "a")
	graph := entities.NewGraph(
		[]entities.Decl{subject, port, double},
		[]entities.Edge{
			needs(double, subject),
			{Dependent: double.Key, Provider: port.Key, Reason: entities.ReasonImplements},
		},
		nil,
	)
	own := owners.Ownership{
		Owner:    map[entities.DeclKey]reach.FeatureID{subject.Key: "a", port.Key: "b"},
		Deferred: reach.Wiring{double.Key: true},
		Roots:    reach.Wiring{},
	}
	born := map[entities.DeclKey]bool{subject.Key: true}
	fresh := map[entities.DeclKey]bool{subject.Key: true}

	got := ReadyDeferred(graph, own, born, fresh)

	if !slices.Contains(got, double.Key) {
		t.Fatalf("readyDeferred() = %v, want the double born before the interface it fills", got)
	}
}
