package format

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBatch(t *testing.T) {
	testCases := []formatCase{
		{
			name: "files mode bisects around one bad file",
			mode: ModeFiles,
			files: []File{
				{"a.go", []byte("a")},
				{"b.go", []byte("BAD")},
				{"c.go", []byte("c")},
				{"d.go", []byte("d")},
			},
			want:   []string{"A", "BAD", "C", "D"},
			failed: []string{"b.go"},
			atMost: 9,
		},
		{
			name: "two bad blobs with one error text do not take down the good ones",
			mode: ModeFiles,
			prepare: func(t *testing.T, _ *Config) {
				t.Helper()
				t.Setenv("FAKEFMT_ANON", "1")
			},
			files: []File{
				{"a.go", []byte("a")},
				{"b.go", []byte("BAD")},
				{"c.go", []byte("c")},
				{"d.go", []byte("BAD")},
			},
			want:   []string{"A", "BAD", "C", "BAD"},
			failed: []string{"b.go", "d.go"},
			atMost: 9,
		},
		{
			name:    "a batch timing out under load still formats its halves",
			mode:    ModeFiles,
			timeout: 3 * time.Second,
			prepare: func(t *testing.T, _ *Config) {
				t.Helper()
				t.Setenv("FAKEFMT_MAXFILES", "2")
			},
			files: numbered(4),
			want:  repeatedWith(4, nil),
			exact: 3,
		},
		{
			name:    "a healthy tool costs one run per batch",
			mode:    ModeFiles,
			prepare: withMutation(func(config *Config) { config.BatchSize, config.Workers = 8, 1 }),
			files:   numbered(64),
			want:    repeatedWith(64, nil),
			atMost:  8,
		},
		{
			name:      "a cancelled context in files mode launches nothing",
			mode:      ModeFiles,
			cancelled: true,
			files:     numbered(20),
			failed:    pathsOf(numbered(20)),
			cause:     context.Canceled,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestBatchOutages(t *testing.T) {
	testCases := []formatCase{
		{
			name: "a dead tool fails every blob with its own exit, never as aborted",
			mode: ModeFiles,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()
				t.Setenv("FAKEFMT_DOWN", "1")

				config.BatchSize, config.Workers = 8, 1
			},
			files:  numbered(64),
			failed: pathsOf(numbered(64)),
			spared: ErrStepAborted,
			exact:  120,
		},
		{
			name: "a dead tool in stdin mode costs one run per blob",
			mode: ModeStdin,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()
				t.Setenv("FAKEFMT_DOWN", "1")

				config.Workers = 1
			},
			files:  numbered(64),
			failed: pathsOf(numbered(64)),
			spared: ErrStepAborted,
			exact:  64,
		},
		{
			name: "a bad first blob and a bad last blob leave the good ones formatted",
			mode: ModeFiles,
			files: []File{
				{"a.go", []byte("BAD")},
				{"b.go", []byte("b")},
				{"c.go", []byte("c")},
				{"d.go", []byte("BAD")},
			},
			want:   []string{"BAD", "B", "C", "BAD"},
			failed: []string{"a.go", "d.go"},
			atMost: 12,
		},
		{
			name:   "a bad blob at position zero and one in the second half",
			mode:   ModeFiles,
			files:  withContent(numbered(8), map[int]string{0: "BAD", 6: "BAD"}),
			want:   []string{"BAD", "X", "X", "X", "X", "X", "BAD", "X"},
			failed: []string{"f0.go", "f6.go"},
		},
		{
			name:    "a pair of bad blobs never marks the tool down",
			mode:    ModeFiles,
			prepare: withMutation(func(config *Config) { config.BatchSize, config.Workers = 2, 1 }),
			files:   withContent(numbered(6), map[int]string{0: "BAD", 1: "BAD", 4: "BAD"}),
			want:    []string{"BAD", "BAD", "X", "X", "BAD", "X"},
			failed:  []string{"f0.go", "f1.go", "f4.go"},
			exact:   7,
		},
		{
			name: "oversized outputs are verdicts of their blobs, never a down tool",
			mode: ModeFiles,
			prepare: withMutation(func(config *Config) {
				config.BatchSize, config.Workers = 3, 1
			}),
			files: withContent(
				numbered(6),
				map[int]string{0: "HUGE", 1: "HUGE", 2: "HUGE", 3: "HUGE"},
			),
			want:   []string{"HUGE", "HUGE", "HUGE", "HUGE", "X", "X"},
			failed: []string{"f0.go", "f1.go", "f2.go", "f3.go"},
			cause:  ErrOutputTooLarge,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func withContent(files []File, contents map[int]string) []File {
	out := slices.Clone(files)
	for index, content := range contents {
		out[index].Content = []byte(content)
	}

	return out
}

func TestBatchNeighbourEvidence(t *testing.T) {
	sharedText := func(t *testing.T, config *Config) {
		t.Helper()
		t.Setenv("FAKEFMT_ANON", "1")

		config.BatchSize, config.Workers = 2, 1
	}

	testCases := []formatCase{
		{
			name:  "a blob emptied only in a shared run is formatted by its solo run",
			mode:  ModeFiles,
			files: withContent(numbered(4), map[int]string{1: "VANISH"}),
			want:  []string{"X", "VANISH", "X", "X"},
			exact: 2,
		},
		{
			name:   "a blob emptied on its solo run fails with the empty output",
			mode:   ModeFiles,
			files:  withContent(numbered(4), map[int]string{1: "EMPTY"}),
			want:   []string{"X", "EMPTY", "X", "X"},
			failed: []string{"f1.go"},
			cause:  ErrEmptyOutput,
			exact:  2,
		},
		{
			name: "a step that allows empty output reruns nothing",
			mode: ModeFiles,
			prepare: withMutation(func(config *Config) {
				config.Languages[0].Steps[0].AllowEmpty = true
			}),
			files: withContent(numbered(4), map[int]string{1: "EMPTY"}),
			want:  []string{"X", "", "X", "X"},
			exact: 1,
		},
		{
			name:    "two bad blobs are no evidence of a dead tool",
			mode:    ModeFiles,
			prepare: sharedText,
			files:   withContent(numbered(4), map[int]string{0: "BAD", 1: "BAD", 2: "BAD"}),
			want:    []string{"BAD", "BAD", "BAD", "X"},
			failed:  []string{"f0.go", "f1.go", "f2.go"},
		},
		{
			name: "two bad blobs spare a good one with several workers",
			mode: ModeFiles,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()
				t.Setenv("FAKEFMT_ANON", "1")

				config.BatchSize, config.Workers = 2, 4
			},
			files: withContent(
				numbered(8),
				map[int]string{0: "BAD", 1: "BAD", 2: "BAD", 4: "BAD", 5: "BAD", 6: "BAD"},
			),
			want:   []string{"BAD", "BAD", "BAD", "X", "BAD", "BAD", "BAD", "X"},
			failed: []string{"f0.go", "f1.go", "f2.go", "f4.go", "f5.go", "f6.go"},
		},
		{
			name: "a mark from a first chunk never fails a good blob of a later one",
			mode: ModeFiles,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()
				t.Setenv("FAKEFMT_ANON", "1")

				config.BatchSize, config.Workers = 4, 1
			},
			files: withContent(
				numbered(8),
				map[int]string{0: "BAD", 1: "BAD", 2: "BAD", 3: "BAD", 4: "BAD"},
			),
			want:   []string{"BAD", "BAD", "BAD", "BAD", "BAD", "X", "X", "X"},
			failed: []string{"f0.go", "f1.go", "f2.go", "f3.go", "f4.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestBatchSilent(t *testing.T) {
	silent := func(batch int) func(*testing.T, *Config) {
		return func(t *testing.T, config *Config) {
			t.Helper()
			t.Setenv("FAKEFMT_SILENT", "1")

			config.BatchSize, config.Workers = batch, 1
		}
	}

	testCases := []formatCase{
		{
			name:    "a chunk of 8 with bad blobs at 0, 4 and 7",
			mode:    ModeFiles,
			prepare: silent(8),
			files:   withContent(numbered(8), map[int]string{0: "BAD", 4: "BAD", 7: "BAD"}),
			want:    repeatedWith(8, map[int]string{0: "BAD", 4: "BAD", 7: "BAD"}),
			failed:  []string{"f0.go", "f4.go", "f7.go"},
			spared:  ErrStepAborted,
		},
		{
			name:    "a chunk of 5 with bad blobs at 0, 2 and 4",
			mode:    ModeFiles,
			prepare: silent(5),
			files:   withContent(numbered(5), map[int]string{0: "BAD", 2: "BAD", 4: "BAD"}),
			want:    repeatedWith(5, map[int]string{0: "BAD", 2: "BAD", 4: "BAD"}),
			failed:  []string{"f0.go", "f2.go", "f4.go"},
			spared:  ErrStepAborted,
		},
		{
			name:    "two chunks of 4 over 8 blobs with bad blobs at 0, 2, 3, 5 and 6",
			mode:    ModeFiles,
			prepare: silent(4),
			files: withContent(
				numbered(8),
				map[int]string{0: "BAD", 2: "BAD", 3: "BAD", 5: "BAD", 6: "BAD"},
			),
			want: repeatedWith(
				8,
				map[int]string{0: "BAD", 2: "BAD", 3: "BAD", 5: "BAD", 6: "BAD"},
			),
			failed: []string{"f0.go", "f2.go", "f3.go", "f5.go", "f6.go"},
			spared: ErrStepAborted,
		},
		{
			name:    "a first chunk of bad blobs fails alone and spares the healthy chunks after it",
			mode:    ModeFiles,
			prepare: silent(4),
			files: withContent(
				numbered(12),
				map[int]string{0: "BAD", 1: "BAD", 2: "BAD", 3: "BAD"},
			),
			want:   repeatedWith(12, map[int]string{0: "BAD", 1: "BAD", 2: "BAD", 3: "BAD"}),
			failed: []string{"f0.go", "f1.go", "f2.go", "f3.go"},
			spared: ErrStepAborted,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestAttempt(t *testing.T) {
	exit := errors.New("exit status 1")
	start := fmt.Errorf("%w: exec: not found", errStart)

	testCases := []struct {
		name        string
		worked      bool
		ended       bool
		outcomes    []error
		wantStarts  int
		wantAborted bool
		wantErr     error
	}{
		{"a clean run starts once", false, false, []error{nil}, 1, false, nil},
		{"an exit failure is never retried", false, false, []error{exit}, 1, false, exit},
		{"a transient start error is retried", false, false, []error{start, nil}, 2, false, nil},
		{"two start errors abort the step", false, false, []error{start, start}, 2, true, errStart},
		{
			"an earlier success does not save it",
			true,
			false,
			[]error{start, start},
			2,
			true,
			errStart,
		},
		{
			"a start error then an exit is no abort",
			false,
			false,
			[]error{start, exit},
			2,
			false,
			exit,
		},
		{"an ended step gets no retry", false, true, []error{start}, 1, true, errStart},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := &stepState{worked: testCase.worked}
			if testCase.ended {
				state.abort(start)
			}

			starts := 0

			got := state.attempt(func() run {
				starts++

				return run{err: testCase.outcomes[min(starts, len(testCase.outcomes))-1]}
			})

			if starts != testCase.wantStarts {
				t.Errorf("starts = %d, want %d", starts, testCase.wantStarts)
			}

			if aborted := state.aborted() != nil; aborted != testCase.wantAborted {
				t.Errorf("aborted = %v, want %v", aborted, testCase.wantAborted)
			}

			if !errors.Is(got.err, testCase.wantErr) {
				t.Errorf("err = %v, want %v", got.err, testCase.wantErr)
			}
		})
	}
}

func TestStepHang(t *testing.T) {
	testCases := []struct {
		name        string
		pilot       int
		worked      bool
		hangs       int
		wantAborted bool
	}{
		{"two solo hangs in a pilot of 8", 8, false, 2, false},
		{"three solo hangs in a pilot of 8", 8, false, 3, true},
		{"two solo hangs fill a pilot of 2 and fail alone", 2, false, 2, false},
		{"one solo hang fills a pilot of 1 and fails alone", 1, false, 1, false},
		{"three solo hangs fill a pilot of 3", 3, false, 3, true},
		{"a success in the pilot spares the step", 8, true, 9, false},
		{"after the pilot a hang is per blob", 0, false, 9, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := &stepState{pilot: testCase.pilot, worked: testCase.worked}

			for range testCase.hangs {
				state.hang(context.DeadlineExceeded)
			}

			if aborted := state.aborted() != nil; aborted != testCase.wantAborted {
				t.Errorf("aborted = %v, want %v", aborted, testCase.wantAborted)
			}
		})
	}
}

func TestSettleCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	state := &stepState{pilot: pilotBlobs, hangs: pilotBlobs - 1}
	part := lane{jobs: []job{{path: "a.go"}}, results: make([]result, 1)}

	(&Engine{}).settle(
		ctx, tool{name: "fakefmt"}, state, part,
		run{err: fmt.Errorf("timed out: %w", context.DeadlineExceeded)},
	)

	if cause := state.aborted(); cause != nil {
		t.Fatalf("the caller's deadline aborted the step: %v", cause)
	}

	got := part.results[0].failure
	if got == nil || !errors.Is(got, context.DeadlineExceeded) || errors.Is(got, ErrStepAborted) {
		t.Errorf("failure = %v, want the deadline and no abort", got)
	}
}

func TestSettleCancelled(t *testing.T) {
	testCases := []struct {
		name string
		jobs int
	}{
		{"a solo run", 1},
		{"a chunk of several", 3},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			part := lane{jobs: make([]job, testCase.jobs), results: make([]result, testCase.jobs)}
			exit := run{err: errors.New("exit status 2"), stderr: "killed"}

			(&Engine{}).settle(ctx, tool{name: "fakefmt"}, &stepState{}, part, exit)

			for position, got := range part.results {
				failure := got.failure
				if failure == nil || !errors.Is(failure, context.Canceled) ||
					failure.Stderr != "" || strings.Contains(failure.Error(), "exit status") {
					t.Errorf(
						"blob %d failure = %+v, want the context error alone",
						position,
						failure,
					)
				}
			}
		})
	}
}
