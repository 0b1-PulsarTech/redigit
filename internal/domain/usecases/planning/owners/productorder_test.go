package owners

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestReadsRootsCountsOnlyWhatAFeatureReadsDirectly(t *testing.T) {
	t.Parallel()

	port := decl("crm/port.go", "Port", "crm")
	shared := decl("kernel/guard.go", "Guard", "kernel")
	handler := decl("routes/handler.go", "Handle", "routes")
	crm := decl("crm/go.mod", "module", "crm")
	kernel := decl("kernel/go.mod", "module", "kernel")
	graph := entities.NewGraph(
		[]entities.Decl{port, shared, handler, crm, kernel},
		[]entities.Edge{
			needs(handler, port), needs(handler, shared), needs(shared, port),
			{Dependent: port.Key, Provider: crm.Key, Reason: entities.ReasonModule},
			{Dependent: shared.Key, Provider: kernel.Key, Reason: entities.ReasonModule},
		},
		nil,
	)
	owner := map[entities.DeclKey]reach.FeatureID{
		port.Key: "crm", handler.Key: "routes", shared.Key: reach.SharedFeature,
	}
	roots := map[string]reach.FeatureID{"crm": "crm", "kernel": "kernel"}

	got := readsRoots(graph, owner, roots)

	if !got["routes"]["crm"] || !got["routes"]["kernel"] || len(got["crm"]) != 0 {
		t.Fatalf("readsRoots() = %v, want routes to wait for crm and kernel", got)
	}

	if len(got[reach.SharedFeature]) != 0 {
		t.Fatalf("readsRoots() = %v, want the shared kernel to wait for nothing", got)
	}
}
