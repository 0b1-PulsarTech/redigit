package verifying

import "testing"

func TestSweepStrayGoFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		subject     string
		modules     []Module
		strays      []string
		exclude     []string
		wantSummary Summary
		wantTail    string
	}{
		{
			"go files with no go.mod are a failure, never empty",
			"s", nil,
			[]string{"main.go"},
			nil,
			Summary{Commits: 1, Checked: 1, Failed: 1},
			"go files outside any go.mod: main.go",
		},
		{
			"a wip commit tolerates them like any failure",
			"[wip] s", nil,
			[]string{"main.go"},
			nil,
			Summary{Commits: 1, Checked: 1, WipFailed: 1},
			"go files outside any go.mod: main.go",
		},
		{
			"they fail beside modules that pass",
			"s", testModules(),
			[]string{"gen/x.go"},
			nil,
			Summary{Commits: 1, Checked: 3, Passed: 2, Failed: 1},
			"go files outside any go.mod: gen/x.go",
		},
		{
			"only the first few paths are named",
			"s", nil,
			[]string{"a/1.go", "b/2.go", "c/3.go", "d/4.go", "e/5.go"},
			nil,
			Summary{Commits: 1, Checked: 1, Failed: 1},
			"go files outside any go.mod: a/1.go, b/2.go, c/3.go (and 2 more)",
		},
		{
			"an excluded directory hides its files",
			"s", nil,
			[]string{"gen/x.go", "gen/deep/y.go"},
			[]string{"gen"},
			Summary{Commits: 1, Empty: 1},
			"",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows, summary := sweepStrays(
				t, testCase.subject, testCase.modules, testCase.strays, testCase.exclude,
			)
			summary.OverheadMicros = 0

			if summary != testCase.wantSummary {
				t.Fatalf("summary = %+v, want %+v", summary, testCase.wantSummary)
			}

			last := rows[len(rows)-1]
			if testCase.wantTail != "" &&
				(last.Step != "module" || last.Tail != testCase.wantTail) {
				t.Fatalf("last row = %+v, want step module and tail %q", last, testCase.wantTail)
			}
		})
	}
}

func sweepStrays(
	t *testing.T,
	subject string,
	mods []Module,
	strays, exclude []string,
) ([]Row, Summary) {
	t.Helper()

	fixture := newRig(1, mods)
	commits := fixture.sweep.Commits.(fakeCommits)
	commits.list[0].Subject = subject

	modules := fixture.sweep.Modules.(fakeModules)
	modules.strays = map[string][]string{"c01": strays}
	fixture.sweep.Modules = modules

	rows, summary, err := collect(fixture.sweep, Options{Exclude: exclude})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	return rows, summary
}
