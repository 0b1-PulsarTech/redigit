package format

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecute(t *testing.T) {
	linkedSecret := func(t *testing.T, _ *Config) {
		t.Helper()

		secret := filepath.Join(t.TempDir(), "secret")
		if err := os.WriteFile(secret, []byte("SECRET"), 0o600); err != nil {
			t.Fatal(err)
		}

		t.Setenv("FAKEFMT_TARGET", secret)
	}

	testCases := []formatCase{
		{
			name:   "stdin mode reports a nonzero exit",
			mode:   ModeStdin,
			files:  []File{{"a.go", []byte("a")}, {"b.go", []byte("BAD")}},
			want:   []string{"A", "BAD"},
			failed: []string{"b.go"},
		},
		{
			name:    "a hanging tool times out and is reported",
			mode:    ModeStdin,
			timeout: 200 * time.Millisecond,
			files:   []File{{"a.go", []byte("HANG")}},
			failed:  []string{"a.go"},
			cause:   context.DeadlineExceeded,
		},
		{
			name:      "a cancelled context in stdin mode launches nothing",
			mode:      ModeStdin,
			cancelled: true,
			files:     numbered(20),
			failed:    pathsOf(numbered(20)),
			cause:     context.Canceled,
		},
		{
			name: "a work dir that cannot be made aborts the step before any run",
			mode: ModeFiles,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()

				missing := filepath.Join(t.TempDir(), "missing")
				for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
					t.Setenv(name, missing)
				}

				config.BatchSize = 2
			},
			files:  numbered(5),
			failed: pathsOf(numbered(5)),
			cause:  ErrStepAborted,
			noRun:  true,
		},
		{
			name:    "a result linked out of the temp dir is refused",
			mode:    ModeFiles,
			prepare: linkedSecret,
			files:   []File{{"a.go", []byte("LINK")}},
			failed:  []string{"a.go"},
		},
		{
			name:   "a path too long to write fails its own blob, not the step",
			mode:   ModeFiles,
			files:  longNamed(numbered(5), 2),
			want:   []string{"X", "X", "x", "X", "X"},
			failed: []string{longNamed(numbered(5), 2)[2].Path},
			spared: ErrStepAborted,
		},
		{
			name:  "stdin writes no file, so a long path is formatted like any other",
			mode:  ModeStdin,
			files: longNamed(numbered(5), 2),
			want:  repeatedWith(5, nil),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestExecuteAnchored(t *testing.T) {
	testCases := []formatCase{
		{
			name: "stdin runs in the config root",
			mode: ModeStdin,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()

				config.ConfigRoot = filepath.Join(t.TempDir(), "cfgroot")
				if err := os.Mkdir(config.ConfigRoot, 0o700); err != nil {
					t.Fatal(err)
				}
			},
			files: []File{{"a.go", []byte("PWD")}},
			want:  []string{"cfgroot"},
		},
		{
			name: "a relative cmd resolves against the config root in files mode",
			mode: ModeFiles,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()

				config.ConfigRoot = t.TempDir()
				installFakeAt(t, config.ConfigRoot)
				config.Languages[0].Steps[0].Cmd = []string{"./bin/fmt", "-w"}
			},
			files: []File{{"a.go", []byte("ab")}},
			want:  []string{"AB"},
		},
		{
			name: "a relative cmd resolves against the config root in stdin mode",
			mode: ModeStdin,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()

				config.ConfigRoot = t.TempDir()
				installFakeAt(t, config.ConfigRoot)
				config.Languages[0].Steps[0].Cmd = []string{"./bin/fmt", "-w"}
			},
			files: []File{{"a.go", []byte("ab")}},
			want:  []string{"AB"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestExecuteLimits(t *testing.T) {
	testCases := []formatCase{
		{
			name:   "a path leaving the repo is refused without a process",
			mode:   ModeFiles,
			files:  []File{{"../escape.go", []byte("x")}},
			failed: []string{"../escape.go"},
			cause:  ErrConfig,
			noRun:  true,
		},
		{
			name:   "an absolute path is refused without a process",
			mode:   ModeFiles,
			files:  []File{{"/abs/x.go", []byte("x")}},
			failed: []string{"/abs/x.go"},
			cause:  ErrConfig,
			noRun:  true,
		},
		{
			name: "a refused path costs its neighbours no extra run",
			mode: ModeFiles,
			files: []File{
				{"a.go", []byte("a")},
				{"b.go", []byte("b")},
				{"../escape.go", []byte("x")},
				{"c.go", []byte("c")},
				{"d.go", []byte("d")},
			},
			want:   []string{"A", "B", "x", "C", "D"},
			failed: []string{"../escape.go"},
			cause:  ErrConfig,
			exact:  1,
		},
		{
			name:   "stdin refuses a huge output and keeps the original",
			mode:   ModeStdin,
			files:  []File{{"a.go", []byte("HUGE")}, {"b.go", []byte("b")}},
			want:   []string{"HUGE", "B"},
			failed: []string{"a.go"},
			cause:  ErrOutputTooLarge,
		},
		{
			name:   "files refuses a huge output and keeps the original",
			mode:   ModeFiles,
			files:  []File{{"a.go", []byte("HUGE")}, {"b.go", []byte("b")}},
			want:   []string{"HUGE", "B"},
			failed: []string{"a.go"},
			cause:  ErrOutputTooLarge,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func longNamed(files []File, index int) []File {
	const nameLimit = 300

	out := slices.Clone(files)
	out[index].Path = strings.Repeat("a", nameLimit) + ".go"

	return out
}

func TestRunErrorArgTooLong(t *testing.T) {
	skipUnlessE2BIG(t)

	testCases := []struct {
		name      string
		err       error
		wantStart bool
		wantBig   bool
	}{
		{"an oversized argument list", &fs.PathError{Err: syscall.E2BIG}, false, true},
		{"a missing binary", &fs.PathError{Err: syscall.ENOENT}, true, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runError(context.Background(), testCase.err)

			if errors.Is(got, errStart) != testCase.wantStart {
				t.Errorf(
					"start error = %v, want %v: %v",
					!testCase.wantStart,
					testCase.wantStart,
					got,
				)
			}

			if errors.Is(got, syscall.E2BIG) != testCase.wantBig {
				t.Errorf("e2big = %v, want %v: %v", !testCase.wantBig, testCase.wantBig, got)
			}
		})
	}
}

func TestArgTooLongFailsBlobsNotTheStep(t *testing.T) {
	skipUnlessE2BIG(t)

	oversized := func(t *testing.T, config *Config) {
		t.Helper()

		const beyondOneArgument = 256 << 10

		cmd := &config.Languages[0].Steps[0].Cmd
		*cmd = append(*cmd, strings.Repeat("x", beyondOneArgument))
	}

	testCases := []formatCase{
		{
			name: "files: a chunk is split down to blobs that fail alone", mode: ModeFiles,
			prepare: oversized, files: numbered(4), failed: pathsOf(numbered(4)),
			cause: syscall.E2BIG, spared: ErrStepAborted, noRun: true,
		},
		{
			name: "stdin: every blob fails alone", mode: ModeStdin,
			prepare: oversized, files: numbered(3), failed: pathsOf(numbered(3)),
			cause: syscall.E2BIG, spared: ErrStepAborted, noRun: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestStartErrorMidBisection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a running executable cannot be unlinked here")
	}

	t.Setenv("FAKEFMT_SELFDESTRUCT", "1")

	log := filepath.Join(t.TempDir(), "invocations")
	t.Setenv("FAKEFMT_LOG", log)

	root := t.TempDir()
	installFakeAt(t, root)

	config := fakeConfig(ModeFiles, time.Minute)
	config.ConfigRoot = root
	config.Languages[0].Steps[0].Cmd = []string{"bin/fmt", "-w"}

	engine, err := New(config)
	if err != nil {
		t.Fatal(err)
	}

	files := withContent(numbered(4), map[int]string{1: "BAD"})

	_, failures := engine.FormatAll(context.Background(), files)
	if len(failures) != len(files) {
		t.Fatalf("failures = %d, want one per blob", len(failures))
	}

	checkAborted(t, failures, allAborted(len(files)))

	if count := invocations(t, log); count != 1 {
		t.Errorf("invocations = %d, want only the first chunk run", count)
	}
}

func skipUnlessE2BIG(t *testing.T) {
	t.Helper()

	if !argTooLong(syscall.E2BIG) {
		t.Skip("this platform does not report an oversized command line as E2BIG")
	}
}

func TestFormatAllSplitsPastTheArgBudget(t *testing.T) {
	const nameWidth = 200

	log := filepath.Join(t.TempDir(), "invocations")
	t.Setenv("FAKEFMT_LOG", log)

	files := make([]File, 2*argBudget/nameWidth)
	for index := range files {
		files[index] = File{Path: fmt.Sprintf("%0*d.go", nameWidth, index), Content: []byte("x")}
	}

	config := fakeConfig(ModeFiles, time.Minute)
	config.BatchSize = len(files)

	engine, err := New(config)
	if err != nil {
		t.Fatal(err)
	}

	_, failures := engine.FormatAll(context.Background(), files)
	if len(failures) != 0 {
		t.Fatalf("failures = %v, want none", failures)
	}

	if count := invocations(t, log); count < 2 {
		t.Errorf("invocations = %d, want the paths split over more than one", count)
	}
}

func TestOnlyPilotCallsHung(t *testing.T) {
	hung := map[int]string{
		0: "HANG", 1: "HANG", 2: "BAD", 3: "HANG", 4: "HANG", 5: "HANG", 6: "HANG",
	}
	given, wanted := maps.Clone(hung), maps.Clone(hung)
	given[7], wanted[7] = "ab", "AB"
	failed := []string{"f0.go", "f1.go", "f2.go", "f3.go", "f4.go", "f5.go", "f6.go"}

	serial := func(t *testing.T, config *Config) {
		t.Helper()

		config.BatchSize, config.Workers = 1, 1
	}

	testCases := []formatCase{
		{
			name:    "files: hangs after a mixed pilot fail per blob",
			mode:    ModeFiles,
			timeout: 100 * time.Millisecond,
			prepare: serial,
			files:   withContent(numbered(8), given),
			want:    repeatedWith(8, wanted),
			failed:  failed,
			spared:  ErrStepAborted,
		},
		{
			name:    "stdin: hangs after a mixed pilot fail per blob",
			mode:    ModeStdin,
			timeout: 100 * time.Millisecond,
			prepare: serial,
			files:   withContent(numbered(8), given),
			want:    repeatedWith(8, wanted),
			failed:  failed,
			spared:  ErrStepAborted,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}
