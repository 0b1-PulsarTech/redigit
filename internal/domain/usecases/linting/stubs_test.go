package linting

import (
	"context"
	"testing"
)

type stubCase struct {
	name   string
	spec   structureSpec
	limits Limits
	want   []Finding
}

func runStubCases(t *testing.T, testCases []stubCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			report, err := CheckStructure(
				context.Background(), structureOf(testCase.spec, syntaxMock{}), testCase.limits,
			)
			if err != nil {
				t.Fatalf("CheckStructure() err = %v", err)
			}

			equalFindings(t, findingsOf(report, MeasureStubGap), testCase.want)

			if got := measureOf(report, MeasureStubGap).Limit; got != testCase.limits.StubGap {
				t.Fatalf("Limit = %d, want %d", got, testCase.limits.StubGap)
			}
		})
	}
}

func TestCheckStructureFindsStubGaps(t *testing.T) {
	t.Parallel()

	runStubCases(t, []stubCase{
		{
			name: "red: the rows of a cut function span more than the limit",
			spec: structureSpec{placed: map[string]int{
				"F": 4, "F#0": 4, "F#1": 25,
			}},
			limits: DefaultLimits(),
			want: []Finding{{
				Measure: MeasureStubGap, Position: 4, Subject: "row 4", Value: 21, Limit: 20,
				Detail: "a.go:func:F: rows 4 to 25, 2 statements",
			}},
		},
		{
			name: "the first row may belong to a statement, not the head",
			spec: structureSpec{placed: map[string]int{
				"F": 30, "F#0": 2, "F#1": 30,
			}},
			limits: DefaultLimits(),
			want: []Finding{{
				Measure: MeasureStubGap, Position: 2, Subject: "row 2", Value: 28, Limit: 20,
				Detail: "a.go:func:F: rows 2 to 30, 2 statements",
			}},
		},
		{
			name: "red: a head far from statements that sit together still counts",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 25, "F#1": 26,
			}},
			limits: DefaultLimits(),
			want: []Finding{{
				Measure: MeasureStubGap, Position: 0, Subject: "row 0", Value: 26, Limit: 20,
				Detail: "a.go:func:F: rows 0 to 26, 2 statements",
			}},
		},
		{
			name: "a limit from the config replaces the default",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 6,
			}},
			limits: Limits{StubGap: 5},
			want: []Finding{{
				Measure: MeasureStubGap, Position: 0, Subject: "row 0", Value: 6, Limit: 5,
				Detail: "a.go:func:F: rows 0 to 6, 2 statements",
			}},
		},
	})
}

func TestCheckStructureLeavesASpanWithinTheLimitAlone(t *testing.T) {
	t.Parallel()

	runStubCases(t, []stubCase{
		{
			name: "green: a span equal to the limit is not over it",
			spec: structureSpec{placed: map[string]int{
				"F": 4, "F#0": 4, "F#1": 24,
			}},
			limits: DefaultLimits(),
		},
		{
			name: "a limit from the config can let a wide span pass",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 30,
			}},
			limits: Limits{StubGap: 40},
		},
		{
			name:   "exclusion: a function that was not cut is one piece in one row",
			spec:   structureSpec{placed: map[string]int{"F": 0, "G": 90}},
			limits: DefaultLimits(),
		},
	})
}
