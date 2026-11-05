package linting

import (
	"context"
	"slices"
	"testing"
)

func TestDeferredNamesTheFiveUnmeasured(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want []string
	}{
		{
			name: "the five a manifest-only run leaves out, under the names its summary has always used",
			want: []string{
				"errcheck_pairs_split", "shells_without_tail", "shells_born_empty",
				"unused_at_birth", "born_as_stubs_gap_over_20_rows",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Deferred(); !slices.Equal(got, testCase.want) {
				t.Fatalf("Deferred() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestEveryDeferredMeasureNowRuns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		spec structureSpec
	}{
		{name: "an empty graph still describes all five", spec: structureSpec{}},
		{
			name: "a graph with a cut function describes the same five",
			spec: structureSpec{placed: map[string]int{"F": 0, "F#0": 0, "F#1": 1}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			report, err := CheckStructure(
				context.Background(), structureOf(testCase.spec, syntaxMock{}), DefaultLimits(),
			)
			if err != nil {
				t.Fatalf("CheckStructure() err = %v", err)
			}

			names := make([]string, 0, len(report.Measures))
			for _, measure := range report.Measures {
				names = append(names, measure.Name)
			}

			want := []string{
				MeasureShellsBornEmpty, MeasureShellsWithoutTail, MeasureStubGap,
				MeasureErrcheckSplit, MeasureUnusedAtBirth,
			}
			if !slices.Equal(names, want) || len(names) != len(Deferred()) {
				t.Fatalf("measures = %v, deferred = %v", names, Deferred())
			}
		})
	}
}
