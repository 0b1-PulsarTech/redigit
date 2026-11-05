package linting

import (
	"context"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type unusedCase struct {
	name       string
	spec       structureSpec
	exempt     map[string]string
	want       []Finding
	wantDetail string
}

func runUnusedCases(t *testing.T, testCases []unusedCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			syntax := syntaxMock{exempt: testCase.exempt}

			report, err := CheckStructure(
				context.Background(), structureOf(testCase.spec, syntax), DefaultLimits(),
			)
			if err != nil {
				t.Fatalf("CheckStructure() err = %v", err)
			}

			equalFindings(t, findingsOf(report, MeasureUnusedAtBirth), testCase.want)

			if got := measureOf(report, MeasureUnusedAtBirth).Detail; !strings.Contains(
				got, testCase.wantDetail,
			) {
				t.Fatalf("Detail = %q, want it to hold %q", got, testCase.wantDetail)
			}
		})
	}
}

func TestCheckStructureFindsDeclarationsUnusedAtBirth(t *testing.T) {
	t.Parallel()

	runUnusedCases(t, []unusedCase{
		{
			name: "red: read only by a declaration born three rows later",
			spec: structureSpec{
				placed: map[string]int{"type:T": 0, "Use": 3},
				edges:  []edgeSpec{{dependent: "Use", provider: "type:T"}},
			},
			want: []Finding{
				{
					Measure: MeasureUnusedAtBirth, Position: 0, Subject: "row 0", Value: 3,
					Detail: "a.go:type:T: first read in row 3", NotCounted: true,
				},
				{
					Measure: MeasureUnusedAtBirth, Position: 3, Subject: "row 3", Value: 0,
					Detail: "a.go:func:Use: never read",
				},
			},
			wantDetail: "measured 2, unused at birth 2 (never read 1; gap of the rest: median 3, max 3)",
		},
		{
			name: "red: nothing reads it at all",
			spec: structureSpec{placed: map[string]int{"helper": 5}},
			want: []Finding{{
				Measure: MeasureUnusedAtBirth, Position: 5, Subject: "row 5", Value: 0,
				Detail: "a.go:func:helper: never read",
			}},
		},
		{
			name: "the first of two later readers sets the gap",
			spec: structureSpec{
				placed: map[string]int{"type:T": 0, "Late": 10, "Early": 3},
				edges: []edgeSpec{
					{dependent: "Late", provider: "type:T"},
					{dependent: "Early", provider: "type:T"},
				},
			},
			exempt: map[string]string{"Late": "a root", "Early": "a root"},
			want: []Finding{{
				Measure: MeasureUnusedAtBirth, Position: 0, Subject: "row 0", Value: 3,
				Detail: "a.go:type:T: first read in row 3", NotCounted: true,
			}},
		},
		{
			name: "a structural edge and a self edge do not read",
			spec: structureSpec{
				placed: map[string]int{"helper": 2, "Mod": 2},
				edges: []edgeSpec{
					{dependent: "Mod", provider: "helper", reason: entities.ReasonHosts},
					{dependent: "helper", provider: "helper"},
				},
			},
			exempt: map[string]string{"Mod": "a root"},
			want: []Finding{{
				Measure: MeasureUnusedAtBirth, Position: 2, Subject: "row 2", Value: 0,
				Detail: "a.go:func:helper: never read",
			}},
		},
	})
}

func TestCheckStructureLeavesAReadDeclarationAlone(t *testing.T) {
	t.Parallel()

	runUnusedCases(t, []unusedCase{
		{
			name: "green: read by a declaration born in the same row",
			spec: structureSpec{
				placed: map[string]int{"type:T": 2, "Use": 2},
				edges:  []edgeSpec{{dependent: "Use", provider: "type:T"}},
			},
			exempt: map[string]string{"Use": "a root"},
		},
		{
			name: "green: read by a declaration born earlier",
			spec: structureSpec{
				placed: map[string]int{"type:T": 4, "Use": 1},
				edges:  []edgeSpec{{dependent: "Use", provider: "type:T"}},
			},
			exempt: map[string]string{"Use": "a root"},
		},
	})
}

func TestCheckStructureCountsWhatItDoesNotMeasure(t *testing.T) {
	t.Parallel()

	runUnusedCases(t, []unusedCase{
		{
			name:       "exclusion: a test is not measured",
			spec:       structureSpec{placed: map[string]int{"TestX": 1}, tests: []string{"TestX"}},
			wantDetail: "excluded: test 1",
		},
		{
			name: "exclusion: generated code is not measured",
			spec: structureSpec{
				placed:    map[string]int{"gen": 1},
				generated: []string{"gen"},
			},
			wantDetail: "excluded: generated 1",
		},
		{
			name:       "exclusion: a method is not measured",
			spec:       structureSpec{placed: map[string]int{"method:T.Run": 1}},
			wantDetail: "excluded: method 1",
		},
		{
			name: "exclusion: a declaration with an implements edge is not measured",
			spec: structureSpec{
				placed: map[string]int{"type:Impl": 1, "type:Iface": 1},
				edges: []edgeSpec{
					{
						dependent: "type:Impl",
						provider:  "type:Iface",
						reason:    entities.ReasonImplements,
					},
				},
			},
			wantDetail: "excluded: implements 2",
		},
		{
			name:       "exclusion: what the language pack exempts is counted by its reason",
			spec:       structureSpec{placed: map[string]int{"main": 1, "Exported": 1}},
			exempt:     map[string]string{"main": "root", "Exported": "exported"},
			wantDetail: "excluded: exported 1, root 1",
		},
		{
			name:       "exclusion: a statement is a piece of a function, not a declaration",
			spec:       structureSpec{placed: map[string]int{"F": 0, "F#0": 0, "F#1": 0}},
			exempt:     map[string]string{"F": "root"},
			wantDetail: "excluded: root 1, statement 2",
		},
		{
			name:       "exclusion: a non-code declaration is not measured",
			spec:       structureSpec{placed: map[string]int{"other:README": 1}},
			wantDetail: "excluded: not code 1",
		},
		{
			name:       "exclusion: a declaration the manifest never births is counted apart",
			spec:       structureSpec{unplaced: []string{"ghost"}},
			wantDetail: "unplaced 1",
		},
	})
}
