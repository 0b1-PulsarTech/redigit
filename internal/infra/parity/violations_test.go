package parity

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestViolations(t *testing.T) {
	t.Parallel()

	plan := []Row{rowOf(0, "dep"), rowOf(1, "prov", "same"), rowOf(1, "dep2"), rowOf(2, "dep3")}

	testCases := []struct {
		name         string
		edges        []Edge
		wantCount    int
		wantTotal    int
		wantExamples int
	}{
		{
			name:         "provider after dependent",
			edges:        []Edge{{Dependent: "dep", Provider: "prov"}},
			wantCount:    1,
			wantTotal:    1,
			wantExamples: 1,
		},
		{
			name:      "provider before dependent",
			edges:     []Edge{{Dependent: "dep3", Provider: "prov"}},
			wantTotal: 1,
		},
		{
			name:      "same commit is fine",
			edges:     []Edge{{Dependent: "dep2", Provider: "same"}},
			wantTotal: 1,
		},
		{
			name:  "endpoint outside plan is excluded",
			edges: []Edge{{Dependent: "dep", Provider: "gone"}},
		},
		{name: "no edges"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Violations(plan, testCase.edges)
			if got.Measure.Value != testCase.wantCount || got.Measure.Total != testCase.wantTotal {
				t.Fatalf("measure = %+v", got.Measure)
			}

			if len(got.Examples) != testCase.wantExamples {
				t.Fatalf("examples = %v", got.Examples)
			}

			if err := got.Measure.Validate(); err != nil {
				t.Fatalf("measure invalid: %v", err)
			}
		})
	}
}

func TestViolationsCountsPairsOnce(t *testing.T) {
	t.Parallel()

	plan := []Row{rowOf(0, "dep"), rowOf(1, "prov")}
	edges := []Edge{
		{Dependent: "dep", Provider: "prov", Reason: entities.ReasonReference},
		{Dependent: "dep", Provider: "prov", Reason: entities.ReasonImport},
		{Dependent: "dep", Provider: "prov", Reason: entities.ReasonReference},
		{Dependent: "dep", Provider: "prov", Reason: entities.ReasonImport},
	}

	got := Violations(plan, edges)
	if got.Measure.Value != 1 || got.Measure.Total != 1 {
		t.Fatalf("measure = %+v, want one distinct pair", got.Measure)
	}

	want := []entities.EdgeReason{entities.ReasonReference, entities.ReasonImport}
	if len(got.Examples) != 1 || !slices.Equal(got.Examples[0].Reasons, want) {
		t.Fatalf("examples = %+v, want reasons %v", got.Examples, want)
	}
}

func TestViolationsCapsExamples(t *testing.T) {
	t.Parallel()

	edges := make([]Edge, 25)
	plan := make([]Row, 0, len(edges)+1)
	plan = append(plan, rowOf(1, "prov"))

	for idx := range edges {
		key := fmt.Sprintf("dep%02d", idx)
		plan = append(plan, rowOf(0, key))
		edges[idx] = Edge{Dependent: key, Provider: "prov"}
	}

	got := Violations(plan, edges)
	if got.Measure.Value != 25 || len(got.Examples) != maxViolationExamples {
		t.Fatalf("count = %d examples = %d", got.Measure.Value, len(got.Examples))
	}

	if example := got.Examples[0]; example.DependentPos != 0 || example.ProviderPos != 1 {
		t.Fatalf("example = %+v", example)
	}
}

func TestViolationsNamesDerivedEdgesAsExcluded(t *testing.T) {
	t.Parallel()

	got := Violations(nil, nil)

	for _, want := range []string{"derived edges", "method-set closure", "distinct"} {
		found := slices.ContainsFunc(got.Measure.Excluded, func(text string) bool {
			return strings.Contains(text, want)
		})
		if !found {
			t.Errorf("Excluded %q does not mention %q", got.Measure.Excluded, want)
		}
	}
}
