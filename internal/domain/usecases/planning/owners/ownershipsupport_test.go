package owners

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

	own := Assign(graph)

	if want := []reach.FeatureID{"lead"}; !slices.Equal(own.Order, want) {
		t.Errorf("order = %v, want %v: test support is no feature to build", own.Order, want)
	}

	if got := own.Owner[checker.Key]; got != reach.SharedFeature {
		t.Errorf("owner of the checker = %q, want it shared", got)
	}

	if got := own.Reach[checker.Key]; got != 0 {
		t.Errorf("reach of the checker = %d, want no built feature to reach it", got)
	}

	if !own.Deferred[checker.Key] {
		t.Error("the checker is not deferred, so it is born before any test reads it")
	}
}
