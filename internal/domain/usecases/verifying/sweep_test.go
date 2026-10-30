package verifying

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func testModules() []Module {
	return []Module{
		{Dir: "alpha", Path: "ex/alpha", Language: "go", Subtree: "a1"},
		{
			Dir:      "beta",
			Path:     "ex/beta",
			Language: "go",
			Subtree:  "b1",
			Requires: []string{"ex/alpha"},
		},
	}
}

func TestSweepRows(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		subject     string
		failing     map[string]string
		modules     []Module
		wantStatus  []Status
		wantSummary Summary
	}{
		{
			"every module passes",
			"s", nil, testModules(),
			[]Status{StatusPass, StatusPass},
			Summary{Commits: 1, Checked: 2, Passed: 2},
		},
		{
			"a failure is reported with its tail",
			"s",
			map[string]string{"c01/beta": "undefined: x"},
			testModules(),
			[]Status{StatusPass, StatusFail},
			Summary{Commits: 1, Checked: 2, Passed: 1, Failed: 1},
		},
		{
			"a wip failure is tolerated",
			"[wip] s",
			map[string]string{"c01/beta": "undefined: x"},
			testModules(),
			[]Status{StatusPass, StatusFail},
			Summary{Commits: 1, Checked: 2, Passed: 1, WipFailed: 1},
		},
		{
			"a commit with no module is empty, never a pass",
			"s", nil, nil,
			[]Status{StatusEmpty},
			Summary{Commits: 1, Empty: 1},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newRig(1, testCase.modules)
			fixture.builder.failing = testCase.failing

			commits := fixture.sweep.Commits.(fakeCommits)
			commits.list[0].Subject = testCase.subject

			rows, summary, err := collect(fixture.sweep, Options{})
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			statuses := make([]Status, 0, len(rows))
			for _, row := range rows {
				statuses = append(statuses, row.Status)
			}

			if !slices.Equal(statuses, testCase.wantStatus) {
				t.Fatalf("statuses = %v, want %v", statuses, testCase.wantStatus)
			}

			summary.OverheadMicros = 0
			if summary != testCase.wantSummary {
				t.Fatalf("summary = %+v, want %+v", summary, testCase.wantSummary)
			}

			for _, row := range rows {
				if row.Status == StatusFail &&
					(row.Tail != "undefined: x" || row.Step != "go build") {
					t.Fatalf("failing row = %+v, want the tail and step", row)
				}
			}
		})
	}
}

func TestSweepCache(t *testing.T) {
	t.Parallel()

	fixture := newRig(3, testModules())
	fixture.builder.failing["c02/beta"] = "bad"

	firstRows, _, err := collect(fixture.sweep, Options{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	if fixture.cache.puts != 5 {
		t.Fatalf("puts = %d, want 5: a failure is never cached", fixture.cache.puts)
	}

	before := len(fixture.builder.checked)

	secondRows, summary, err := collect(fixture.sweep, Options{})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if rechecked := len(fixture.builder.checked) - before; rechecked != 1 {
		t.Fatalf("rechecked = %d, want only the failing module", rechecked)
	}

	if summary.Cached != 5 || summary.Failed != 1 || len(secondRows) != len(firstRows) {
		t.Fatalf(
			"summary = %+v rows = %d, want 5 cache hits and the failure again",
			summary,
			len(secondRows),
		)
	}
}

func TestSweepSkipsCheckoutWhenEveryModuleHits(t *testing.T) {
	t.Parallel()

	fixture := newRig(4, testModules())

	if _, _, err := collect(fixture.sweep, Options{}); err != nil {
		t.Fatalf("cold run: %v", err)
	}

	calls := fixture.trees.calls

	if _, _, err := collect(fixture.sweep, Options{}); err != nil {
		t.Fatalf("warm run: %v", err)
	}

	if fixture.trees.calls != calls {
		t.Fatalf("checkouts grew from %d to %d on a fully cached run", calls, fixture.trees.calls)
	}
}

func TestSweepExclude(t *testing.T) {
	t.Parallel()

	modules := append(
		[]Module{{Dir: "gen/lifted", Path: "ex/lifted", Subtree: "g1"}},
		testModules()...)
	fixture := newRig(1, modules)

	testCases := []struct {
		name    string
		exclude []string
		want    []string
	}{
		{"nothing excluded", nil, []string{"alpha", "beta", "gen/lifted"}},
		{"a directory prefix", []string{"gen"}, []string{"alpha", "beta"}},
		{"an exact module dir", []string{"alpha"}, []string{"beta", "gen/lifted"}},
		{"a trailing slash", []string{"gen/"}, []string{"alpha", "beta"}},
		{"a trailing slash on a module dir", []string{"alpha/"}, []string{"beta", "gen/lifted"}},
		{
			"a sibling prefix is not a match",
			[]string{"ge"},
			[]string{"alpha", "beta", "gen/lifted"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows, _, err := collect(fixture.sweep, Options{Exclude: testCase.exclude})
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			dirs := make([]string, 0, len(rows))
			for _, row := range rows {
				dirs = append(dirs, row.Module)
			}

			if !slices.Equal(dirs, testCase.want) {
				t.Fatalf("modules = %v, want %v", dirs, testCase.want)
			}
		})
	}
}

func TestSweepClosesTheSlots(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		commitsErr error
		checkErr   error
		closeErr   error
		wantErr    bool
	}{
		{"a clean run", nil, nil, nil, false},
		{"a run whose build failed", nil, errBoom, nil, true},
		{"a run that could not list commits", errBoom, nil, nil, true},
		{"a close failure does not fail the run", nil, nil, errBoom, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newRig(2, testModules())
			fixture.builder.checkErr = testCase.checkErr
			fixture.trees.closeErr = testCase.closeErr
			fixture.sweep.Commits = fakeCommits{
				list: fixture.sweep.Commits.(fakeCommits).list, err: testCase.commitsErr,
			}

			_, _, err := collect(fixture.sweep, Options{})
			if (err != nil) != testCase.wantErr {
				t.Fatalf("run err = %v, want error %v", err, testCase.wantErr)
			}

			if fixture.trees.closes != 1 {
				t.Fatalf("closed %d times, want once", fixture.trees.closes)
			}
		})
	}
}

func TestSweepFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		edit    func(fixture *rig)
		opts    Options
		wantErr error
		isBuild bool
	}{
		{"the commit list fails", func(fixture *rig) {
			fixture.sweep.Commits = fakeCommits{err: errBoom}
		}, Options{}, errBoom, false},
		{"the module list fails", func(fixture *rig) {
			fixture.sweep.Modules = fakeModules{err: errBoom}
		}, Options{}, errBoom, false},
		{
			"the checkout fails",
			func(fixture *rig) { fixture.trees.err = errBoom },
			Options{},
			errBoom,
			false,
		},
		{"the builder cannot run its tool", func(fixture *rig) {
			fixture.builder.checkErr = errBoom
		}, Options{}, ErrBuilder, true},
		{"the builder cannot name its identity", func(fixture *rig) {
			fixture.builder.identityErr = errBoom
		}, Options{}, ErrBuilder, true},
		{"the options are invalid", func(*rig) {}, Options{Stride: -1}, ErrInvalid, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newRig(3, testModules())
			testCase.edit(fixture)

			rows, _, err := collect(fixture.sweep, testCase.opts)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if errors.Is(err, ErrBuilder) != testCase.isBuild {
				t.Fatalf(
					"builder error = %v, want %v",
					errors.Is(err, ErrBuilder),
					testCase.isBuild,
				)
			}

			for _, row := range rows {
				if row.Status == StatusPass {
					t.Fatalf("row %+v reported green on an aborted sweep", row)
				}
			}
		})
	}
}

func TestSweepCacheTrouble(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		edit func(cache *fakeCache)
	}{
		{"a cache read error is a miss", func(cache *fakeCache) { cache.getErr = errBoom }},
		{
			"a cache write error does not fail the run",
			func(cache *fakeCache) { cache.putErr = errBoom },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newRig(2, testModules())
			testCase.edit(fixture.cache)

			_, summary, err := collect(fixture.sweep, Options{})
			if err != nil || summary.Passed != 4 || summary.Cached != 0 {
				t.Fatalf("summary = %+v err = %v, want 4 fresh passes", summary, err)
			}
		})
	}
}

func TestSweepCancellation(t *testing.T) {
	t.Parallel()

	fixture := newRig(6, testModules())
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	t.Cleanup(cancel)

	fixture.builder.onCheck = func(ctx context.Context) error {
		close(started)
		<-ctx.Done()

		return ctx.Err()
	}

	errs := make(chan error, 1)

	go func() {
		_, err := fixture.sweep.Run(ctx, Options{}, func(Row) error { return nil })
		errs <- err
	}()

	<-started
	cancel()

	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSweepEmitFailureAborts(t *testing.T) {
	t.Parallel()

	fixture := newRig(5, testModules())

	_, err := fixture.sweep.Run(context.Background(), Options{Parallel: 2}, func(Row) error {
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the emit error", err)
	}
}

func TestSweepOrderAndSlots(t *testing.T) {
	t.Parallel()

	var baseline []Row

	for _, parallel := range []int{1, 4} {
		fixture := newRig(12, testModules())
		fixture.builder.failing["c05/alpha"] = "bad"

		rows, summary, err := collect(fixture.sweep, Options{Parallel: parallel})
		if err != nil {
			t.Fatalf("parallel %d: %v", parallel, err)
		}

		if summary.Commits != 12 || len(rows) != 24 {
			t.Fatalf("parallel %d: commits = %d rows = %d", parallel, summary.Commits, len(rows))
		}

		for idx := 1; idx < len(rows); idx++ {
			if rows[idx].Position < rows[idx-1].Position {
				t.Fatalf("parallel %d: rows out of position order at %d", parallel, idx)
			}
		}

		for slot, positions := range fixture.trees.slots {
			isContiguous := positions[len(positions)-1]-positions[0] == len(positions)-1
			if !slices.IsSorted(positions) || !isContiguous || slot >= parallel {
				t.Fatalf(
					"parallel %d: slot %d took %v, want one contiguous range",
					parallel,
					slot,
					positions,
				)
			}
		}

		for idx := range rows {
			rows[idx].Millis = 0
		}

		if baseline == nil {
			baseline = rows

			continue
		}

		if !slices.Equal(rows, baseline) {
			t.Fatalf("parallel %d rows differ from parallel 1", parallel)
		}
	}
}
