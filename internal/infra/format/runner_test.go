package format

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRunner(t *testing.T) {
	testCases := []formatCase{
		{
			name: "files mode formats all in one run",
			mode: ModeFiles,
			files: []File{
				{"a/x.go", []byte("ab")},
				{"b/y.go", []byte("cd")},
				{"c/z.go", []byte("ef")},
			},
			want:  []string{"AB", "CD", "EF"},
			exact: 1,
		},
		{
			name:  "stdin mode runs one process per blob",
			mode:  ModeStdin,
			files: []File{{"a.go", []byte("ab")}, {"b.go", []byte("cd")}},
			want:  []string{"AB", "CD"},
			exact: 2,
		},
		{
			name:    "a batch size caps each run",
			mode:    ModeFiles,
			prepare: withMutation(func(config *Config) { config.BatchSize = 3 }),
			files:   numbered(7),
			want:    repeatedWith(7, nil),
			exact:   3,
		},
		{
			name:   "stdin refuses an emptied blob",
			mode:   ModeStdin,
			files:  []File{{"a.go", []byte("EMPTY")}},
			failed: []string{"a.go"},
			cause:  ErrEmptyOutput,
		},
		{
			name:   "files refuses an emptied blob",
			mode:   ModeFiles,
			files:  []File{{"a.go", []byte("EMPTY")}},
			failed: []string{"a.go"},
			cause:  ErrEmptyOutput,
		},
		{
			name: "stdin allows an emptied blob when the step opts in",
			mode: ModeStdin,
			prepare: withMutation(func(config *Config) {
				config.Languages[0].Steps[0].AllowEmpty = true
			}),
			files: []File{{"a.go", []byte("EMPTY")}},
			want:  []string{""},
		},
		{
			name: "files allows an emptied blob when the step opts in",
			mode: ModeFiles,
			prepare: withMutation(func(config *Config) {
				config.Languages[0].Steps[0].AllowEmpty = true
			}),
			files: []File{{"a.go", []byte("EMPTY")}},
			want:  []string{""},
		},
		{
			name:  "an empty blob stays empty",
			mode:  ModeStdin,
			files: []File{{"a.go", nil}},
			want:  []string{""},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestStartOutage(t *testing.T) {
	testCases := []struct {
		name         string
		mode         Mode
		timeout      time.Duration
		remove       bool
		files        []File
		wantAborted  []bool
		wantContents []string
	}{
		{
			name:        "stdin: a missing binary aborts every blob",
			mode:        ModeStdin,
			timeout:     5 * time.Second,
			remove:      true,
			files:       numbered(6),
			wantAborted: allAborted(6),
		},
		{
			name:        "files: a missing binary aborts every blob of every chunk",
			mode:        ModeFiles,
			timeout:     5 * time.Second,
			remove:      true,
			files:       numbered(100),
			wantAborted: allAborted(100),
		},
		{
			name:         "stdin: three hung blobs in the pilot abort the healthy fourth",
			mode:         ModeStdin,
			timeout:      300 * time.Millisecond,
			files:        withContent(numbered(4), map[int]string{0: "HANG", 1: "HANG", 2: "HANG"}),
			wantAborted:  []bool{false, false, true, true},
			wantContents: []string{"HANG", "HANG", "HANG", "x"},
		},
		{
			name:         "stdin: a hang after a pilot success fails alone",
			mode:         ModeStdin,
			timeout:      2 * time.Second,
			files:        withContent(numbered(6), map[int]string{1: "HANG", 3: "HANG", 4: "HANG"}),
			wantAborted:  []bool{false, false, false},
			wantContents: []string{"X", "HANG", "X", "HANG", "HANG", "X"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			installFakeAt(t, root)

			config := fakeConfig(testCase.mode, testCase.timeout)
			config.Workers, config.BatchSize, config.ConfigRoot = 4, 8, root
			config.Languages[0].Steps[0].Cmd = []string{"bin/fmt"}

			engine, err := New(config)
			if err != nil {
				t.Fatal(err)
			}

			if testCase.remove {
				if err = os.Remove(filepath.Join(root, "bin", "fmt")); err != nil {
					t.Fatal(err)
				}
			}

			got, failures := engine.FormatAll(context.Background(), testCase.files)
			if len(failures) != len(testCase.wantAborted) {
				t.Fatalf("failures = %d, want %d", len(failures), len(testCase.wantAborted))
			}

			checkAborted(t, failures, testCase.wantAborted)

			for index, want := range testCase.wantContents {
				if string(got[index].Content) != want {
					t.Errorf("file %d = %q, want %q", index, got[index].Content, want)
				}
			}
		})
	}
}

func checkAborted(t *testing.T, failures []*Failure, want []bool) {
	t.Helper()

	for index, failure := range failures {
		aborted := errors.Is(failure, ErrStepAborted)
		if aborted != want[index] {
			t.Errorf("failure %d aborted = %v, want %v", index, aborted, want[index])
		}

		if want[index] && !strings.Contains(failure.Error(), "step aborted: ") {
			t.Errorf("failure %d does not say why: %v", index, failure)
		}
	}
}

func allAborted(count int) []bool {
	out := make([]bool, count)
	for index := range out {
		out[index] = true
	}

	return out
}

func repeatedWith(count int, overrides map[int]string) []string {
	out := make([]string, count)
	for index := range out {
		out[index] = "X"
	}

	for index, content := range overrides {
		out[index] = content
	}

	return out
}

func TestPilotHungTool(t *testing.T) {
	hung := func(batch int) func(*testing.T, *Config) {
		return func(t *testing.T, config *Config) {
			t.Helper()
			t.Setenv("FAKEFMT_HANG", "1")

			config.BatchSize, config.Workers = batch, 4
		}
	}

	testCases := []formatCase{
		{
			name: "files: 8 blobs", mode: ModeFiles, timeout: 100 * time.Millisecond,
			prepare: hung(8), files: numbered(8), failed: pathsOf(numbered(8)),
			cause: ErrStepAborted, settled: 2, exact: 7,
		},
		{
			name: "files: 64 blobs", mode: ModeFiles, timeout: 100 * time.Millisecond,
			prepare: hung(8), files: numbered(64), failed: pathsOf(numbered(64)),
			cause: ErrStepAborted, settled: 2, exact: 7,
		},
		{
			name: "files: 1000 blobs", mode: ModeFiles, timeout: 100 * time.Millisecond,
			prepare: hung(8), files: numbered(1000), failed: pathsOf(numbered(1000)),
			cause: ErrStepAborted, settled: 2, exact: 7,
		},
		{
			name: "stdin: 8 blobs", mode: ModeStdin, timeout: 100 * time.Millisecond,
			prepare: hung(8), files: numbered(8), failed: pathsOf(numbered(8)),
			cause: ErrStepAborted, settled: 2, exact: 3,
		},
		{
			name: "stdin: 1000 blobs", mode: ModeStdin, timeout: 100 * time.Millisecond,
			prepare: hung(8), files: numbered(1000), failed: pathsOf(numbered(1000)),
			cause: ErrStepAborted, settled: 2, exact: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestPilotHangsAlone(t *testing.T) {
	const patient = 2 * time.Second

	pairs := withMutation(func(config *Config) { config.BatchSize, config.Workers = 2, 2 })

	testCases := []formatCase{
		{
			name:     "a hanging blob inside the pilot chunk fails alone",
			mode:     ModeFiles,
			timeout:  patient,
			parallel: true,
			prepare:  pairs,
			files:    withContent(numbered(4), map[int]string{1: "HANG"}),
			want:     repeatedWith(4, map[int]string{1: "HANG"}),
			failed:   []string{"f1.go"},
			cause:    context.DeadlineExceeded,
			spared:   ErrStepAborted,
		},
		{
			name:     "two hanging blobs in one chunk after a good pilot fail alone",
			mode:     ModeFiles,
			timeout:  patient,
			parallel: true,
			prepare:  pairs,
			files:    withContent(numbered(6), map[int]string{4: "HANG", 5: "HANG"}),
			want:     repeatedWith(6, map[int]string{4: "HANG", 5: "HANG"}),
			failed:   []string{"f4.go", "f5.go"},
			cause:    context.DeadlineExceeded,
			spared:   ErrStepAborted,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestPilotDeterminism(t *testing.T) {
	testCases := []struct {
		name    string
		mode    Mode
		env     map[string]string
		timeout time.Duration
		batch   int
		files   []File
	}{
		{
			name:    "a hung tool",
			mode:    ModeFiles,
			env:     map[string]string{"FAKEFMT_HANG": "1"},
			timeout: 100 * time.Millisecond,
			batch:   8,
			files:   numbered(64),
		},
		{
			name:    "a hung tool in stdin mode",
			mode:    ModeStdin,
			env:     map[string]string{"FAKEFMT_HANG": "1"},
			timeout: 100 * time.Millisecond,
			batch:   8,
			files:   numbered(40),
		},
		{
			name:    "a silent tool with scattered bad blobs",
			mode:    ModeFiles,
			env:     map[string]string{"FAKEFMT_SILENT": "1"},
			timeout: 5 * time.Second,
			batch:   4,
			files: withContent(
				numbered(24),
				map[int]string{0: "BAD", 2: "BAD", 3: "BAD", 9: "BAD", 17: "BAD", 23: "BAD"},
			),
		},
		{
			name:    "a dead tool",
			mode:    ModeStdin,
			env:     map[string]string{"FAKEFMT_DOWN": "1"},
			timeout: 5 * time.Second,
			batch:   4,
			files:   numbered(16),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for key, value := range testCase.env {
				t.Setenv(key, value)
			}

			var outcomes [2][]string

			for slot, workers := range []int{1, 4} {
				config := fakeConfig(testCase.mode, testCase.timeout)
				config.BatchSize, config.Workers = testCase.batch, workers

				engine, err := New(config)
				if err != nil {
					t.Fatal(err)
				}

				_, failures := engine.FormatAll(context.Background(), testCase.files)
				for _, failure := range failures {
					outcomes[slot] = append(outcomes[slot], failure.Path+": "+failure.Err.Error())
				}
			}

			if !slices.Equal(outcomes[0], outcomes[1]) {
				t.Errorf("workers 1 and 4 disagree:\n%v\n%v", outcomes[0], outcomes[1])
			}

			if len(outcomes[0]) == 0 {
				t.Error("the scenario failed nothing")
			}
		})
	}
}

func TestPilotThreshold(t *testing.T) {
	testCases := []thresholdCase{
		{"files: one hung blob fails alone", ModeFiles, 8, nil, numbered(1), []bool{false}, 1},
		{
			"files: two hung blobs fail alone",
			ModeFiles, 8, nil, numbered(2),
			[]bool{false, false},
			3,
		},
		{
			"files: three hung blobs abort at the third",
			ModeFiles, 8, nil, numbered(3),
			[]bool{false, false, true},
			5,
		},
		{"stdin: one hung blob fails alone", ModeStdin, 8, nil, numbered(1), []bool{false}, 1},
		{
			"stdin: two hung blobs fail alone",
			ModeStdin, 8, nil, numbered(2),
			[]bool{false, false},
			2,
		},
		{
			"stdin: three hung blobs abort at the third",
			ModeStdin, 8, nil, numbered(3),
			[]bool{false, false, true},
			3,
		},
		{
			"files: an isolated first blob does not call the hang alone",
			ModeFiles, 8,
			[]string{"f0.go"},
			numbered(8),
			[]bool{false, false, true, true, true, true, true, true},
			6,
		},
		{
			"files: three hung blobs spread over pilot chunks abort at the third",
			ModeFiles, 1, nil, numbered(5),
			[]bool{false, false, true, true, true},
			3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

type thresholdCase struct {
	name        string
	mode        Mode
	batch       int
	isolate     []string
	files       []File
	wantAborted []bool
	exact       int
}

func (testCase thresholdCase) run(t *testing.T) {
	t.Helper()

	log := filepath.Join(t.TempDir(), "invocations")
	t.Setenv("FAKEFMT_LOG", log)
	t.Setenv("FAKEFMT_HANG", "1")

	config := fakeConfig(testCase.mode, 50*time.Millisecond)
	config.BatchSize, config.Workers = testCase.batch, 4
	config.Languages[0].Isolate = testCase.isolate

	engine, err := New(config)
	if err != nil {
		t.Fatal(err)
	}

	_, failures := engine.FormatAll(context.Background(), testCase.files)
	if len(failures) != len(testCase.files) {
		t.Fatalf("failures = %d, want %d", len(failures), len(testCase.files))
	}

	for index, failure := range failures {
		if !errors.Is(failure, context.DeadlineExceeded) {
			t.Errorf("failure %d = %v, want a timeout", index, failure)
		}
	}

	checkAborted(t, failures, testCase.wantAborted)

	if count := invocations(t, log); count != testCase.exact {
		t.Errorf("invocations = %d, want %d", count, testCase.exact)
	}
}
