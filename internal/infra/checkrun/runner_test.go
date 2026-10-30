package checkrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func fake(args ...string) Step {
	return Step{Argv: append([]string{"fakecheck"}, args...), Timeout: time.Minute}
}

func runnerFor(t *testing.T, steps ...Step) (*Runner, string) {
	t.Helper()

	slotDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(slotDir, "mod"), 0o750); err != nil {
		t.Fatalf("module dir: %v", err)
	}

	runner := mustRunner(t, Config{
		Languages: map[string]Language{"go": {Steps: steps}}, CacheDir: t.TempDir(),
	})

	return runner, slotDir
}

func goUnit() verifying.Unit {
	return verifying.Unit{Module: verifying.Module{Dir: "mod", Path: "ex/mod", Language: "go"}}
}

type outcomeCase struct {
	name       string
	steps      func(log string) []Step
	wantFailed bool
	wantStep   string
	wantTail   string
	wantLog    []string
}

func TestCheckTolerates(t *testing.T) {
	t.Parallel()

	checkOutcomes(t, []outcomeCase{
		{
			"a failure carrying the tolerated text passes and the next step runs",
			func(log string) []Step {
				tolerated := fake("fail", log, "no packages to vet")
				tolerated.Tolerate = []string{"no packages to vet"}

				return []Step{tolerated, fake("pass", log)}
			},
			false, "fakecheck pass", "",
			[]string{"fail", "pass"},
		},
		{
			"a failure without the tolerated text still fails",
			func(log string) []Step {
				tolerated := fake("fail", log, "undefined: x")
				tolerated.Tolerate = []string{"no packages to vet"}

				return []Step{tolerated, fake("pass", log)}
			},
			true, "fakecheck fail", "undefined: x",
			[]string{"fail"},
		},
		{
			"every output line being a tolerated one passes",
			func(log string) []Step {
				tolerated := fake("fail", log, "warning: none\nno packages to vet")
				tolerated.Tolerate = []string{"no packages to vet", "warning: none"}

				return []Step{tolerated, fake("pass", log)}
			},
			false, "fakecheck pass", "",
			[]string{"fail", "pass"},
		},
		{
			"a real error line beside the tolerated text still fails",
			func(log string) []Step {
				tolerated := fake("fail", log, "undefined: x\nno packages to vet")
				tolerated.Tolerate = []string{"no packages to vet"}

				return []Step{tolerated, fake("pass", log)}
			},
			true, "fakecheck fail", "undefined: x",
			[]string{"fail"},
		},
		{
			"the tolerated text with another exit code still fails",
			func(log string) []Step {
				tolerated := fake("crash", log, "no packages to vet")
				tolerated.Tolerate = []string{"no packages to vet"}

				return []Step{tolerated, fake("pass", log)}
			},
			true, "fakecheck crash", "no packages to vet",
			[]string{"crash"},
		},
	})
}

func TestCheckOutcomes(t *testing.T) {
	t.Parallel()

	checkOutcomes(t, []outcomeCase{
		{
			"every step passes in order",
			func(log string) []Step { return []Step{fake("pass", log), fake("pass", log)} },
			false, "fakecheck pass", "",
			[]string{"pass", "pass"},
		},
		{
			"the first failure stops the rest and carries its stderr",
			func(log string) []Step {
				return []Step{
					fake("pass", log),
					fake("fail", log, "undefined: x"),
					fake("pass", log),
				}
			},
			true, "fakecheck fail", "undefined: x",
			[]string{"pass", "fail"},
		},
		{
			"a flood keeps only the tail",
			func(string) []Step { return []Step{fake("flood")} },
			true, "fakecheck flood", "line 3999 of the flood", nil,
		},
		{
			"a step past its timeout fails the commit",
			func(string) []Step {
				return []Step{
					{Argv: []string{"fakecheck", "hang"}, Timeout: 200 * time.Millisecond},
				}
			},
			true, "fakecheck hang", "timed out", nil,
		},
	})
}

func checkOutcomes(t *testing.T, testCases []outcomeCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			log := filepath.Join(t.TempDir(), "log")
			runner, slotDir := runnerFor(t, testCase.steps(log)...)

			outcome, err := runner.Check(context.Background(), slotDir, goUnit())
			if err != nil {
				t.Fatalf("check: %v", err)
			}

			if outcome.IsFailed != testCase.wantFailed || outcome.Step != testCase.wantStep ||
				!strings.Contains(outcome.Tail, testCase.wantTail) {
				t.Fatalf("outcome = %+v, want failed=%v step=%q tail~%q",
					outcome, testCase.wantFailed, testCase.wantStep, testCase.wantTail)
			}

			if len(outcome.Tail) > 9<<10 {
				t.Fatalf("tail is %d bytes, want it bounded", len(outcome.Tail))
			}

			assertLog(t, log, slotDir, testCase.wantLog)
		})
	}
}

func assertLog(t *testing.T, log, slotDir string, want []string) {
	t.Helper()

	if want == nil {
		return
	}

	content, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	var got []string

	for line := range strings.SplitSeq(strings.TrimSpace(string(content)), "\n") {
		what, cwd, _ := strings.Cut(line, " ")
		if resolved, _ := filepath.EvalSymlinks(
			cwd,
		); resolved != mustResolve(
			t,
			filepath.Join(slotDir, "mod"),
		) {
			t.Fatalf("step ran in %q, want the module dir", cwd)
		}

		got = append(got, what)
	}

	if !slices.Equal(got, want) {
		t.Fatalf("steps run = %v, want %v", got, want)
	}
}

func mustResolve(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}

	return resolved
}

func TestCheckBuilderErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		steps   []Step
		unit    verifying.Unit
		wantErr error
	}{
		{
			"a tool that is not installed",
			[]Step{{Argv: []string{"redigit-no-such-tool"}, Timeout: time.Minute}},
			goUnit(), ErrToolMissing,
		},
		{"a language with no steps", nil, goUnit(), ErrNoSteps},
		{
			"a language nobody configured",
			[]Step{fake("pass")},
			verifying.Unit{Module: verifying.Module{Dir: "mod", Language: "cobol"}},
			ErrNoSteps,
		},
		{"an empty argv", []Step{{Timeout: time.Minute}}, goUnit(), ErrNoSteps},
		{
			"a module dir the slot does not hold",
			[]Step{fake("pass")},
			verifying.Unit{Module: verifying.Module{Dir: "gone", Language: "go"}},
			ErrModuleDir,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner, slotDir := runnerFor(t, testCase.steps...)

			outcome, err := runner.Check(context.Background(), slotDir, testCase.unit)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if outcome.IsFailed {
				t.Fatalf(
					"outcome = %+v, a builder error must not look like a check failure",
					outcome,
				)
			}
		})
	}
}

func TestCheckCancellation(t *testing.T) {
	t.Parallel()

	runner, slotDir := runnerFor(t, fake("hang"))
	ctx, cancel := context.WithCancel(context.Background())

	time.AfterFunc(200*time.Millisecond, cancel)

	_, err := runner.Check(ctx, slotDir, goUnit())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled and no outcome", err)
	}
}

func mustRunner(t *testing.T, cfg Config) *Runner {
	t.Helper()

	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(string(filepath.Separator), "redigit-identity-only")
	}

	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	return runner
}

func TestNew(t *testing.T) {
	testCases := []struct {
		name     string
		cacheDir string
		wantErr  error
		wantAbs  bool
	}{
		{"an empty cache dir", "", ErrNoCacheDir, false},
		{"a relative cache dir", "rel", nil, true},
		{"a nested relative cache dir", filepath.Join("a", "..", "b", "rel"), nil, true},
		{"an absolute cache dir", filepath.Join(string(filepath.Separator), "abs"), nil, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			runner, err := New(Config{CacheDir: testCase.cacheDir})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantAbs && !filepath.IsAbs(runner.cfg.CacheDir) {
				t.Fatalf("cache dir = %q, want an absolute path", runner.cfg.CacheDir)
			}
		})
	}
}

func TestCheckRootsTheGoCacheAtTheAbsoluteCacheDir(t *testing.T) {
	t.Chdir(t.TempDir())

	dump := filepath.Join(t.TempDir(), "env")
	runner := mustRunner(t, Config{
		Languages: map[string]Language{"go": {Steps: []Step{fake("env", dump)}}}, CacheDir: "rel",
	})

	slotDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(slotDir, "mod"), 0o750); err != nil {
		t.Fatalf("module dir: %v", err)
	}

	outcome, err := runner.Check(t.Context(), slotDir, goUnit())
	if err != nil || outcome.IsFailed {
		t.Fatalf("check: outcome = %+v err = %v", outcome, err)
	}

	content, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}

	for line := range strings.Lines(string(content)) {
		if value, found := strings.CutPrefix(line, "GOCACHE="); found &&
			!filepath.IsAbs(strings.TrimSpace(value)) {
			t.Fatalf("GOCACHE = %q, want an absolute path", value)
		}
	}
}
