package planning

import (
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// A test waits for the member of its atom that needs the last feature. The
// feature freshens that member alone, and nothing is left to freshen the test:
// it is born once the last feature is built, not in the tail of what no entry
// point reached.
func TestPlannerBirthsATestWaitingOnItsAtomAfterTheLastFeature(t *testing.T) {
	t.Parallel()

	handler := decl("a/handler.go", "Handle", "a")
	counted := decl("a/count.go", "Count", "a")
	late := decl("b/handler.go", "Serve", "b")
	early := testDecl("b/early_test.go", "TestEarly", "b")
	last := testDecl("b/last_test.go", "TestLast", "b")
	early.Atom, last.Atom = "e2e-main", "e2e-main"

	graph := entities.NewGraph(
		[]entities.Decl{handler, counted, late, early, last},
		[]entities.Edge{needs(handler, counted), needs(early, counted), needs(last, late)},
		nil,
	)
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		handler.Key: 0, counted.Key: 1, late.Key: 2, early.Key: 3, last.Key: 4,
	}}

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	for _, commit := range plan.Commits {
		if strings.HasPrefix(commit.Reason, "reached by no entry point") {
			t.Fatalf("commit %q holds what no feature reached", commit.Subject)
		}
	}
}
