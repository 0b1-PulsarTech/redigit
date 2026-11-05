package linting

import (
	"context"
	"errors"
	"strings"
	"testing"
)

var errBroken = errors.New("broken read")

type errsplitCase struct {
	name       string
	spec       structureSpec
	statements map[string]Statement
	want       []Finding
	wantDetail string
}

func runErrsplitCases(t *testing.T, testCases []errsplitCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			syntax := syntaxMock{statements: testCase.statements}

			report, err := CheckStructure(
				context.Background(), structureOf(testCase.spec, syntax), DefaultLimits(),
			)
			if err != nil {
				t.Fatalf("CheckStructure() err = %v", err)
			}

			equalFindings(t, findingsOf(report, MeasureErrcheckSplit), testCase.want)

			if got := measureOf(report, MeasureErrcheckSplit).Detail; !strings.Contains(
				got, testCase.wantDetail,
			) {
				t.Fatalf("Detail = %q, want it to hold %q", got, testCase.wantDetail)
			}
		})
	}
}

func TestCheckStructureNamesWhatEachSplitPairDoesToErr(t *testing.T) {
	t.Parallel()

	assign := Statement{Assign: FormAssign}
	define := Statement{Assign: FormDefine}
	check := Statement{Checks: true}

	runErrsplitCases(t, []errsplitCase{
		{
			name: "red: err = and its check land in different rows",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 1,
			}},
			statements: map[string]Statement{"F#0": assign, "F#1": check},
			want: []Finding{{
				Measure: MeasureErrcheckSplit, Position: 0, Subject: "row 0", Value: 1,
				Detail: "a.go:func:F: plain err = at statement 0 in row 0, " +
					"checked at statement 1 in row 1",
			}},
			wantDetail: "pairs 1, split 1 (plain err =: 1; " +
				"err already bound, with other names defined: 0; err newly bound here: 0)",
		},
		{
			name: "red: a := after an earlier binding reuses err",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 0, "F#2": 2, "F#3": 5,
			}},
			statements: map[string]Statement{
				"F#0": define, "F#1": check, "F#2": define, "F#3": check,
			},
			want: []Finding{{
				Measure: MeasureErrcheckSplit, Position: 2, Subject: "row 2", Value: 3,
				Detail: "a.go:func:F: err already bound, with other names defined at statement 2 " +
					"in row 2, checked at statement 3 in row 5",
			}},
			wantDetail: "pairs 2, split 1 (plain err =: 0; " +
				"err already bound, with other names defined: 1; err newly bound here: 0)",
		},
		{
			name: "red: a := that no earlier statement bound introduces err",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 2, "F#1": 5,
			}},
			statements: map[string]Statement{"F#0": define, "F#1": check},
			want: []Finding{{
				Measure: MeasureErrcheckSplit, Position: 2, Subject: "row 2", Value: 3,
				Detail: "a.go:func:F: err newly bound here at statement 0 in row 2, " +
					"checked at statement 1 in row 5",
			}},
			wantDetail: "pairs 1, split 1 (plain err =: 0; " +
				"err already bound, with other names defined: 0; err newly bound here: 1)",
		},
		{
			name: "red: a var err T before a := makes the := a reuse",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 1, "F#2": 2,
			}},
			statements: map[string]Statement{
				"F#0": {Binds: true}, "F#1": define, "F#2": check,
			},
			want: []Finding{{
				Measure: MeasureErrcheckSplit, Position: 1, Subject: "row 1", Value: 1,
				Detail: "a.go:func:F: err already bound, with other names defined at statement 1 " +
					"in row 1, checked at statement 2 in row 2",
			}},
			wantDetail: "pairs 1, split 1 (plain err =: 0; " +
				"err already bound, with other names defined: 1; err newly bound here: 0)",
		},
	})
}

func TestCheckStructurePairsAnAssignmentWithItsNextCheck(t *testing.T) {
	t.Parallel()

	assign := Statement{Assign: FormAssign}
	check := Statement{Checks: true}
	none := "pairs 0, split 0 (plain err =: 0; " +
		"err already bound, with other names defined: 0; err newly bound here: 0)"

	runErrsplitCases(t, []errsplitCase{
		{
			name: "green: the pair shares a row",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 3, "F#1": 3,
			}},
			statements: map[string]Statement{"F#0": assign, "F#1": check},
			wantDetail: "pairs 1, split 0 (plain err =: 0;",
		},
		{
			name: "exclusion: an assignment overwritten before a check pairs with nothing",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 1, "F#2": 1,
			}},
			statements: map[string]Statement{"F#0": assign, "F#1": assign, "F#2": check},
			wantDetail: "pairs 1, split 0 (plain err =: 0;",
		},
		{
			name: "exclusion: an assignment nothing checks pairs with nothing",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 4,
			}},
			statements: map[string]Statement{"F#0": assign},
			wantDetail: none,
		},
		{
			name: "exclusion: a check before the assignment pairs with nothing",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 4,
			}},
			statements: map[string]Statement{"F#0": check, "F#1": assign},
			wantDetail: none,
		},
		{
			name: "red: a statement the reader returns no fact for is counted as unread",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 4,
			}},
			statements: map[string]Statement{"F#0": assign},
			wantDetail: "statements unread 1",
		},
		{
			name: "exclusion: a language with no statement reader finds nothing and reads none",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 4,
			}},
			wantDetail: none + "; statements unread 0",
		},
	})
}

func TestCheckStructureRefusesAnUnreadableSource(t *testing.T) {
	t.Parallel()

	spec := structureSpec{placed: map[string]int{"F": 0, "F#0": 0, "F#1": 1}}

	_, err := CheckStructure(
		context.Background(), structureOf(spec, syntaxMock{err: errBroken}), DefaultLimits(),
	)
	if !errors.Is(err, errBroken) {
		t.Fatalf("CheckStructure() err = %v, want %v", err, errBroken)
	}
}
