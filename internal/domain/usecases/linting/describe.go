package linting

func describe(limits Limits) []Measure {
	const (
		growth = "net line growth of the path over the last row that held it, summed over "

		directories = "vendor/, node_modules/, testdata/ and dist/ count by their extension and " +
			"marker only, not by directory"

		generatedFiles = "generated files: a Go file with the Code generated marker, any other " +
			"file with generated in its first line, go.sum, atlas.sum, package-lock.json, " +
			"pnpm-lock.yaml, .min.js"
	)

	return []Measure{
		{
			Name:       MeasureSourceAdded,
			Definition: growth + "the source files of one row, strictly over the limit",
			Limit:      limits.SourceAdded,
			Excluded: append(
				caveats(),
				directories,
				generatedFiles,
				"files that are not source code (docs, config, data)",
				"source files recognised by extension only: go dart ts tsx js jsx svelte sql py rs",
			),
		},
		{
			Name: MeasureGeneratedMixed,
			Definition: growth + "the generated files of one row that also grew source, " +
				"strictly over the limit",
			Limit: limits.GeneratedMixed,
			Excluded: append(
				caveats(),
				directories,
				generatedFiles,
				"rows that grew generated files only",
				"a row asks whether it grew source: a source file rewritten at equal length does not",
			),
		},
		{
			Name:       MeasureRowAdded,
			Definition: growth + "every path of one row, strictly over the limit",
			Limit:      limits.RowAdded,
			Excluded:   caveats(),
		},
	}
}

func caveats() []string {
	return []string{
		"net growth only: a rewrite at equal length adds 0 and a partial rewrite adds only " +
			"its growth",
		"lines removed: a file that shrinks adds nothing and offsets no other file",
		"renames: a moved file counts as new",
		"submodule entries",
		"binary files (a blob holding a NUL byte): they count 0 lines",
	}
}
