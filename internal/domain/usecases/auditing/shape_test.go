package auditing

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestAuditorCountsOnlyDeclarationsWithAReader(t *testing.T) {
	t.Parallel()

	graph, order, _ := wiring()

	result, err := New(Config{}).Audit(graph, order)
	if err != nil {
		t.Fatalf("Audit() = %v", err)
	}

	near, found := measureNamed(result.Report.Shape, "born within")
	if !found {
		t.Fatal("no proximity measure")
	}

	// Two of the five declarations have a reader outside their unit, and one
	// of those two drifts. Counting the readerless three as successes would
	// report 4 of 5 and make the tree look better than it is.
	if near.Total != 2 {
		t.Fatalf("total = %d, want the 2 declarations something outside reads", near.Total)
	}

	if near.Value != 1 {
		t.Fatalf("value = %d, want 1 of 2 near their reader", near.Value)
	}
}

func TestAuditorMeasuresTestsOnTheFunction(t *testing.T) {
	t.Parallel()

	subject := decl("lead/lead.go", "Create", "lead")
	together := decl("lead/lead_test.go", "TestCreate", "lead")
	late := decl("lead/other_test.go", "TestOther", "lead")
	other := decl("lead/other.go", "Other", "lead")

	graph := entities.NewGraph(
		[]entities.Decl{subject, together, late, other},
		nil,
		[]entities.Anchor{
			{Decl: together.Key, To: subject.Key, Reason: entities.ReasonTestOf},
			{Decl: late.Key, To: other.Key, Reason: entities.ReasonTestOf},
		},
	)

	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		subject.Key:  10,
		together.Key: 10,
		other.Key:    20,
		late.Key:     600,
	}}

	result, err := New(Config{}).Audit(graph, order)
	if err != nil {
		t.Fatalf("Audit() = %v", err)
	}

	tests, found := measureNamed(result.Report.Shape, "tests born in the same commit")
	if !found {
		t.Fatal("no test-pairing measure")
	}

	if tests.Total != 2 || tests.Value != 1 {
		t.Fatalf("tests = %d of %d, want 1 of 2", tests.Value, tests.Total)
	}
}
