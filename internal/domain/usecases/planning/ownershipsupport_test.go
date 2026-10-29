package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestAssignBuildsNoFeatureForCodeOnlyTestsRead(t *testing.T) {
	t.Parallel()

	checker := decl("apperrtest/check.go", "Require", "apperrtest")
	handler := decl("lead/handler.go", "Handle", "lead")
	leadTest := testDecl("lead/handler_test.go", "TestHandle", "lead")
	graph := entities.NewGraph(
		[]entities.Decl{checker, handler, leadTest},
		[]entities.Edge{needs(leadTest, checker), needs(leadTest, handler)},
		nil,
	)

	own := assign(graph)

	if want := []FeatureID{"lead"}; !slices.Equal(own.order, want) {
		t.Errorf("order = %v, want %v: test support is no feature to build", own.order, want)
	}

	if got := own.owner[checker.Key]; got != reach.SharedFeature {
		t.Errorf("owner of the checker = %q, want it shared", got)
	}

	if got := own.reach[checker.Key]; got != 0 {
		t.Errorf("reach of the checker = %d, want no built feature to reach it", got)
	}

	if !own.deferred[checker.Key] {
		t.Error("the checker is not deferred, so it is born before any test reads it")
	}
}
