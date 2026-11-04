package linting

import (
	"maps"
	"slices"
	"testing"
)

func TestCheckFindsEachMeasure(t *testing.T) {
	t.Parallel()

	defaults := DefaultLimits()

	testCases := []struct {
		name   string
		limits Limits
		rows   []Row
		want   []Finding
	}{
		{
			name:   "red: source over the limit",
			limits: defaults,
			rows: []Row{{Position: 3, Subject: "s", Files: []File{
				{Path: "a.go", Added: 901, Kind: KindSource},
			}}},
			want: []Finding{
				{Measure: MeasureSourceAdded, Position: 3, Subject: "s", Value: 901, Limit: 900},
				{Measure: MeasureRowAdded, Position: 3, Subject: "s", Value: 901, Limit: 850},
			},
		},
		{
			name:   "green: source at the limit adds up to rows over only",
			limits: defaults,
			rows: []Row{{Position: 1, Subject: "s", Files: []File{
				{Path: "a.go", Added: 900, Kind: KindSource},
			}}},
			want: []Finding{
				{Measure: MeasureRowAdded, Position: 1, Subject: "s", Value: 900, Limit: 850},
			},
		},
		{
			name:   "red: generated mixed with source",
			limits: defaults,
			rows: []Row{{Position: 2, Subject: "g", Files: []File{
				{Path: "db/q.sql.go", Added: 901, Kind: KindGenerated},
				{Path: "main.go", Added: 1, Kind: KindSource},
			}}},
			want: []Finding{
				{Measure: MeasureGeneratedMixed, Position: 2, Subject: "g", Value: 901, Limit: 900},
				{Measure: MeasureRowAdded, Position: 2, Subject: "g", Value: 902, Limit: 850},
			},
		},
		{
			name:   "green: generated alone is not mixed",
			limits: defaults,
			rows: []Row{{Position: 2, Subject: "g", Files: []File{
				{Path: "db/q.sql.go", Added: 901, Kind: KindGenerated},
			}}},
			want: []Finding{
				{Measure: MeasureRowAdded, Position: 2, Subject: "g", Value: 901, Limit: 850},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Check(testCase.rows, testCase.limits)
			if !slices.Equal(got.Findings, testCase.want) {
				t.Fatalf("Findings = %+v, want %+v", got.Findings, testCase.want)
			}
		})
	}
}

func TestCheckCountsOnlyGrowthOfKnownKinds(t *testing.T) {
	t.Parallel()

	defaults := DefaultLimits()

	testCases := []struct {
		name   string
		limits Limits
		rows   []Row
		want   []Finding
	}{
		{
			name:   "red: rows over with docs only",
			limits: defaults,
			rows: []Row{{Position: 5, Subject: "d", Files: []File{
				{Path: "README.md", Added: 851, Kind: KindOther},
			}}},
			want: []Finding{
				{Measure: MeasureRowAdded, Position: 5, Subject: "d", Value: 851, Limit: 850},
			},
		},
		{
			name:   "green: rows at the limit",
			limits: defaults,
			rows: []Row{{Position: 5, Subject: "d", Files: []File{
				{Path: "README.md", Added: 850, Kind: KindOther},
			}}},
		},
		{
			name:   "a file of another kind is not source",
			limits: defaults,
			rows: []Row{{Position: 7, Subject: "y", Files: []File{
				{Path: "notes.go.txt", Added: 2000, Kind: KindOther},
			}}},
			want: []Finding{
				{Measure: MeasureRowAdded, Position: 7, Subject: "y", Value: 2000, Limit: 850},
			},
		},
		{
			name:   "a negative delta adds nothing and offsets no other file",
			limits: defaults,
			rows: []Row{{Position: 6, Subject: "x", Files: []File{
				{Path: "a.go", Added: 1000, Kind: KindSource},
				{Path: "b.go", Added: -500, Kind: KindSource},
			}}},
			want: []Finding{
				{Measure: MeasureSourceAdded, Position: 6, Subject: "x", Value: 1000, Limit: 900},
				{Measure: MeasureRowAdded, Position: 6, Subject: "x", Value: 1000, Limit: 850},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Check(testCase.rows, testCase.limits)
			if !slices.Equal(got.Findings, testCase.want) {
				t.Fatalf("Findings = %+v, want %+v", got.Findings, testCase.want)
			}
		})
	}
}

func TestCheckKeepsEachLimitApart(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		limits Limits
		rows   []Row
		want   []Finding
	}{
		{
			name:   "the source limit alone does not judge generated lines",
			limits: Limits{SourceAdded: 100, GeneratedMixed: 500, RowAdded: 10000},
			rows: []Row{{Position: 8, Subject: "m", Files: []File{
				{Path: "q.sql.go", Added: 300, Kind: KindGenerated},
				{Path: "a.go", Added: 1, Kind: KindSource},
			}}},
		},
		{
			name:   "the generated limit alone judges generated lines",
			limits: Limits{SourceAdded: 500, GeneratedMixed: 100, RowAdded: 10000},
			rows: []Row{{Position: 9, Subject: "m", Files: []File{
				{Path: "q.sql.go", Added: 300, Kind: KindGenerated},
				{Path: "a.go", Added: 1, Kind: KindSource},
			}}},
			want: []Finding{
				{Measure: MeasureGeneratedMixed, Position: 9, Subject: "m", Value: 300, Limit: 100},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Check(testCase.rows, testCase.limits)
			if !slices.Equal(got.Findings, testCase.want) {
				t.Fatalf("Findings = %+v, want %+v", got.Findings, testCase.want)
			}
		})
	}
}

func TestCheckCountsAndUsesTheGivenLimits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		limits     Limits
		rows       []Row
		wantRows   int
		wantCounts map[string]int
	}{
		{
			name:   "two rows over the lowered source limit",
			limits: Limits{SourceAdded: 10, GeneratedMixed: 10, RowAdded: 1000},
			rows: []Row{
				{Position: 0, Files: []File{{Path: "a.go", Added: 11, Kind: KindSource}}},
				{Position: 1, Files: []File{{Path: "b.go", Added: 11, Kind: KindSource}}},
			},
			wantRows: 2,
			wantCounts: map[string]int{
				MeasureSourceAdded: 2, MeasureGeneratedMixed: 0, MeasureRowAdded: 0,
			},
		},
		{
			name:   "one row counts under each measure it trips",
			limits: Limits{SourceAdded: 10, GeneratedMixed: 10, RowAdded: 10},
			rows: []Row{{Position: 0, Files: []File{
				{Path: "a.go", Added: 11, Kind: KindSource},
				{Path: "q.sql.go", Added: 11, Kind: KindGenerated},
			}}},
			wantRows: 1,
			wantCounts: map[string]int{
				MeasureSourceAdded: 1, MeasureGeneratedMixed: 1, MeasureRowAdded: 1,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Check(testCase.rows, testCase.limits)

			counts := map[string]int{}
			for _, measure := range got.Measures {
				counts[measure.Name] = measure.Count
			}

			if got.Rows != testCase.wantRows || !maps.Equal(counts, testCase.wantCounts) {
				t.Fatalf("Rows = %d, counts = %v, want %v", got.Rows, counts, testCase.wantCounts)
			}
		})
	}
}
