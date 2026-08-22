package auditing

import (
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func measureNamed(measures []entities.Measure, prefix string) (entities.Measure, bool) {
	for _, measure := range measures {
		if len(measure.Name) >= len(prefix) && measure.Name[:len(prefix)] == prefix {
			return measure, true
		}
	}

	return entities.Measure{}, false
}

func TestAuditorReportsIntegrityAndShapeApart(t *testing.T) {
	t.Parallel()

	graph, order, _ := wiring()

	result, err := New(Config{}).Audit(graph, order)
	if err != nil {
		t.Fatalf("Audit() = %v", err)
	}

	// Replay validity and grouping quality are two different measurements, and
	// a rebuild can ace the first while failing the second. Folding them into
	// one number is what hides that.
	if len(result.Report.Integrity) == 0 || len(result.Report.Shape) == 0 {
		t.Fatalf("report = %+v, want both kinds populated", result.Report)
	}

	placed, found := measureNamed(result.Report.Integrity, "declarations placed")
	if !found {
		t.Fatal("no placement measure")
	}

	if placed.Value != placed.Total || placed.Total != graph.Len() {
		t.Fatalf("placed = %d of %d, want all %d", placed.Value, placed.Total, graph.Len())
	}
}

// TestEveryMeasureNamesItsBlindSpot is the rule this package exists to keep.
// Eleven bad numbers in the previous rebuild came from the measurer rather
// than the tree, every one of them because nothing recorded what the number
// did not look at.
func TestEveryMeasureNamesItsBlindSpot(t *testing.T) {
	t.Parallel()

	graph, order, _ := wiring()

	result, err := New(Config{}).Audit(graph, order)
	if err != nil {
		t.Fatalf("Audit() = %v", err)
	}

	all := append(append([]entities.Measure{}, result.Report.Integrity...), result.Report.Shape...)
	if len(all) == 0 {
		t.Fatal("no measures at all")
	}

	for _, measure := range all {
		if len(measure.Excluded) == 0 {
			t.Fatalf("measure %q does not say what it excludes", measure.Name)
		}
	}
}

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

func TestAuditorRefusesAnEmptyGraph(t *testing.T) {
	t.Parallel()

	_, err := New(Config{}).Audit(entities.NewGraph(nil, nil, nil), entities.Ordering{})
	if !errors.Is(err, apperr.ErrIncomplete) {
		t.Fatalf("Audit() = %v, want %v", err, apperr.ErrIncomplete)
	}
}

func TestConfigDefaults(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   Config
		want int
	}{
		{"zero takes the default", Config{}, defaultFarApart},
		{"negative takes the default", Config{FarApart: -3}, defaultFarApart},
		{"an explicit value is kept", Config{FarApart: 5}, 5},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.in.withDefaults().FarApart; got != testCase.want {
				t.Fatalf("FarApart = %d, want %d", got, testCase.want)
			}
		})
	}
}
