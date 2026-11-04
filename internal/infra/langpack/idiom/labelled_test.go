package idiom

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type measuringGate struct{ measure entities.Measure }

func (gate measuringGate) Name() string { return gate.measure.Name }

func (gate measuringGate) Measure(entities.Graph, entities.Ordering) entities.Measure {
	return gate.measure
}

func TestLabelled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		inner    entities.Measure
		label    string
		wantName string
	}{
		{
			name:     "the label leads the measure name",
			inner:    entities.Measure{Name: "readiness", Value: 1, Total: 2},
			label:    "go-di",
			wantName: "go-di: readiness",
		},
		{
			name:     "an empty label still separates the name",
			inner:    entities.Measure{Name: "readiness"},
			label:    "",
			wantName: ": readiness",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gate := labelled{
				inner: measuringGate{measure: testCase.inner}, label: testCase.label,
			}
			got := gate.Measure(entities.Graph{}, entities.Ordering{})

			if got.Name != testCase.wantName {
				t.Fatalf("Measure().Name = %q, want %q", got.Name, testCase.wantName)
			}

			if got.Value != testCase.inner.Value || got.Total != testCase.inner.Total {
				t.Fatalf("Measure() = %+v, want the inner counts of %+v", got, testCase.inner)
			}

			if gate.Name() != testCase.inner.Name {
				t.Fatalf("Name() = %q, want %q", gate.Name(), testCase.inner.Name)
			}
		})
	}
}
