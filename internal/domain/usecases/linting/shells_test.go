package linting

import (
	"context"
	"strings"
	"testing"
)

type shellCase struct {
	name         string
	spec         structureSpec
	wantEmpty    []Finding
	wantTail     []Finding
	wantFirstOff string
}

func runShellCases(t *testing.T, testCases []shellCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			report, err := CheckStructure(
				context.Background(), structureOf(testCase.spec, syntaxMock{}), DefaultLimits(),
			)
			if err != nil {
				t.Fatalf("CheckStructure() err = %v", err)
			}

			equalFindings(t, findingsOf(report, MeasureShellsBornEmpty), testCase.wantEmpty)
			equalFindings(t, findingsOf(report, MeasureShellsWithoutTail), testCase.wantTail)

			detail := measureOf(report, MeasureShellsWithoutTail).Detail
			if !strings.Contains(detail, testCase.wantFirstOff) {
				t.Fatalf("Detail = %q, want it to hold %q", detail, testCase.wantFirstOff)
			}
		})
	}
}

func TestShellsWithoutTailDoesNotClaimOnlyVoidFunctionsAreFound(t *testing.T) {
	t.Parallel()

	measure := describeWithoutTail(1, 0)
	text := measure.Definition + " " + strings.Join(measure.Excluded, " ")

	for _, want := range []string{"returned literal", "by element"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Definition and Excluded = %q, want them to say %q", text, want)
		}
	}

	if strings.Contains(text, "cannot have its last statement away") {
		t.Fatalf("Definition and Excluded = %q, still claim a result function is never found", text)
	}
}

func TestCheckStructureFindsAHeadBornEmpty(t *testing.T) {
	t.Parallel()

	runShellCases(t, []shellCase{
		{
			name: "red: the head's row holds none of its statements",
			spec: structureSpec{placed: map[string]int{
				"F": 3, "F#0": 5, "F#1": 5, "F#2": 6,
			}},
			wantEmpty: []Finding{{
				Measure: MeasureShellsBornEmpty, Position: 3, Subject: "row 3", Value: 3,
				Detail: "a.go:func:F: head in row 3, 3 statements in rows 5 to 6",
			}},
			wantTail: []Finding{{
				Measure: MeasureShellsWithoutTail, Position: 3, Subject: "row 3", Value: 3,
				Detail: "a.go:func:F: head in row 3, last statement in row 6 of 3",
			}},
			wantFirstOff: "first statement not in the head's row: 1 of 1",
		},
		{
			name: "green: the head and every statement share a row",
			spec: structureSpec{placed: map[string]int{
				"F": 2, "F#0": 2, "F#1": 2,
			}},
			wantFirstOff: "first statement not in the head's row: 0 of 1",
		},
		{
			name:         "exclusion: a function the slicer did not cut has no shell",
			spec:         structureSpec{placed: map[string]int{"F": 3, "G": 9}},
			wantFirstOff: "first statement not in the head's row: 0 of 0",
		},
	})
}

func TestCheckStructureFindsAShellWithoutATail(t *testing.T) {
	t.Parallel()

	runShellCases(t, []shellCase{
		{
			name: "red: the head holds its first statement but not the last",
			spec: structureSpec{placed: map[string]int{
				"F": 2, "F#0": 2, "F#1": 4,
			}},
			wantTail: []Finding{{
				Measure: MeasureShellsWithoutTail, Position: 2, Subject: "row 2", Value: 2,
				Detail: "a.go:func:F: head in row 2, last statement in row 4 of 2",
			}},
			wantFirstOff: "first statement not in the head's row: 0 of 1",
		},
		{
			name: "green: the head holds the last statement and not the first",
			spec: structureSpec{placed: map[string]int{
				"F": 2, "F#0": 1, "F#1": 2,
			}},
			wantFirstOff: "first statement not in the head's row: 1 of 1",
		},
		{
			name: "a statement is ordered by its span, not by its name",
			spec: structureSpec{placed: map[string]int{
				"F": 2, "F#0": 2, "F#10": 4, "F#9": 2,
			}},
			wantTail: []Finding{{
				Measure: MeasureShellsWithoutTail, Position: 2, Subject: "row 2", Value: 2,
				Detail: "a.go:func:F: head in row 2, last statement in row 4 of 3",
			}},
			wantFirstOff: "first statement not in the head's row: 0 of 1",
		},
		{
			name: "green: a middle statement away does not make a tail, the last is the one read",
			spec: structureSpec{placed: map[string]int{
				"F": 2, "F#0": 2, "F#1": 7, "F#2": 2,
			}},
			wantFirstOff: "first statement not in the head's row: 0 of 1",
		},
	})
}
