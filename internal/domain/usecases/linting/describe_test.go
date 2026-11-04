package linting

import (
	"slices"
	"testing"
)

type describeCase struct {
	name           string
	measure        string
	wantLimit      int
	wantDefinition string
	wantExcluded   []string
}

const (
	growth = "net line growth of the path over the last row that held it, summed over "
	tail   = ", strictly over the limit"

	netGrowth = "net growth only: a rewrite at equal length adds 0 and a partial rewrite " +
		"adds only its growth"
	removed = "lines removed: a file that shrinks adds nothing and offsets no other file"
	renames = "renames: a moved file counts as new"
	modules = "submodule entries"
	binary  = "binary files (a blob holding a NUL byte): they count 0 lines"
)

func checkDescribed(t *testing.T, testCases []describeCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var got Measure

			limits := Limits{SourceAdded: 11, GeneratedMixed: 22, RowAdded: 33}
			for _, measure := range describe(limits) {
				if measure.Name == testCase.measure {
					got = measure
				}
			}

			if got.Limit != testCase.wantLimit || got.Definition != testCase.wantDefinition ||
				!slices.Equal(got.Excluded, testCase.wantExcluded) {
				t.Fatalf("measure = %+v", got)
			}
		})
	}
}

func TestDescribeListsEveryCaveatOfTheSourceMeasures(t *testing.T) {
	t.Parallel()

	dirs := "vendor/, node_modules/, testdata/ and dist/ count by their extension and " +
		"marker only, not by directory"
	generated := "generated files: a Go file with the Code generated marker, any other file " +
		"with generated in its first line, go.sum, atlas.sum, package-lock.json, " +
		"pnpm-lock.yaml, .min.js"

	checkDescribed(t, []describeCase{
		{
			name:           "source added",
			measure:        MeasureSourceAdded,
			wantLimit:      11,
			wantDefinition: growth + "the source files of one row" + tail,
			wantExcluded: []string{
				netGrowth, removed, renames, modules, binary, dirs, generated,
				"files that are not source code (docs, config, data)",
				"source files recognised by extension only: go dart ts tsx js jsx svelte sql py rs",
			},
		},
		{
			name:           "generated mixed",
			measure:        MeasureGeneratedMixed,
			wantLimit:      22,
			wantDefinition: growth + "the generated files of one row that also grew source" + tail,
			wantExcluded: []string{
				netGrowth,
				removed,
				renames,
				modules,
				binary,
				dirs,
				generated,
				"rows that grew generated files only",
				"a row asks whether it grew source: a source file rewritten at equal length does not",
			},
		},
	})
}

func TestDescribeListsEveryCaveatOfRowsOver(t *testing.T) {
	t.Parallel()

	checkDescribed(t, []describeCase{
		{
			name:           "rows over",
			measure:        MeasureRowAdded,
			wantLimit:      33,
			wantDefinition: growth + "every path of one row" + tail,
			wantExcluded:   []string{netGrowth, removed, renames, modules, binary},
		},
	})
}
