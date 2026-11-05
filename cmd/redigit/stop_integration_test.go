//go:build integration

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli"
)

type stopScene struct {
	source string
	blobs  string
	state  string
	cache  string
	rev    string
}

func (scene stopScene) config(t *testing.T, policy string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), policy+".json")
	settings := `{"write": {"identity": {"name": "proof", "email": "proof@localhost"}},` +
		`"lineage": {"policy": "` + policy + `", "anchors": [{"rev": "` + scene.rev +
		`", "kind": "fix"}]}}`

	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

func (scene stopScene) cached() []string {
	entries, _ := filepath.Glob(filepath.Join(scene.cache, "redigit", "plancache", "*.gob"))

	return entries
}

func (scene stopScene) emit(config string, extra ...string) []string {
	return slices.Concat([]string{
		"emit", "--repo", scene.source, "--mode", "lineage", "--config", config,
		"--blob-dir", scene.blobs, "--from-cache",
	}, extra)
}

func newStopScene(t *testing.T) stopScene {
	t.Helper()

	const drop = "fix(aaa): drop the M call"

	scratch := t.TempDir()
	scene := stopScene{
		source: filepath.Join(scratch, "source"),
		blobs:  filepath.Join(scratch, "blobs"),
		state:  filepath.Join(scratch, "state"),
		cache:  filepath.Join(scratch, "xdg"),
	}

	if err := os.MkdirAll(scene.source, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	gitText(t, scene.source, "init", "-q", "-b", "main")
	gitText(t, scene.source, "config", "user.email", "tester@example.com")
	gitText(t, scene.source, "config", "user.name", "Tester")

	aaa := "package aaa\n\nimport \"example.com/p/internal/bbb\"\n\n"
	for _, step := range []historyStep{
		{subject: "feat(aaa): add F", files: map[string]string{
			"go.mod":            modFile,
			"internal/bbb/b.go": methodsTypeOnly,
			methodsDropFile:     aaa + "func F(t bbb.T) int { return 1 }\n" + fixPad,
		}},
		{subject: "feat(bbb): add M", files: map[string]string{
			"internal/bbb/b.go": methodsWithM,
			methodsDropFile:     aaa + "func F(t bbb.T) int { return t.M() }\n" + fixPad,
		}},
		{subject: drop, files: map[string]string{
			methodsDropFile: aaa + "func F(t bbb.T) int { return 1 }\n" + fixPad,
		}},
		{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
	} {
		commitFiles(t, scene.source, step.subject, step.files)
	}

	scene.rev = gitText(
		t, scene.source, "log", "--format=%H", "--fixed-strings", "--grep="+drop, "-1",
	)

	t.Setenv("XDG_CACHE_HOME", scene.cache)

	return scene
}

func runCode(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	var out, errs bytes.Buffer

	code = run(context.WithoutCancel(t.Context()), args, &out, &errs, cli.Run)

	return code, out.String(), errs.String()
}

func TestStopKeepsItsStateAndWritesNothingToStdoutOrTheCache(t *testing.T) {
	scene := newStopScene(t)
	config := scene.config(t, "stop")

	code, stdout, stderr := runCode(t, scene.emit(config, "--state-dir", scene.state)...)
	if code != 3 {
		t.Fatalf("stop run = %d, want 3\n%s", code, stderr)
	}

	if stdout != "" {
		t.Fatalf("a stop run wrote %d bytes to stdout, want none", len(stdout))
	}

	for _, want := range []string{
		"state.json", filepath.Join("conflicts", scene.rev),
	} {
		if _, err := os.Stat(filepath.Join(scene.state, want)); err != nil {
			t.Fatalf("state is missing %s: %v", want, err)
		}
	}

	if held := scene.cached(); len(held) != 0 {
		t.Fatalf("the plan cache holds %v after a stopped run", held)
	}

	if code, _, stderr = runCode(t, scene.emit(scene.config(t, "fold"))...); code != 0 {
		t.Fatalf("control run = %d\n%s", code, stderr)
	}

	if held := scene.cached(); len(held) != 1 {
		t.Fatalf("a settled run left %v in the plan cache, want one entry", held)
	}
}

func TestStopWithoutAStateDirIsAUsageError(t *testing.T) {
	scene := newStopScene(t)

	code, stdout, _ := runCode(t, scene.emit(scene.config(t, "stop"))...)
	if code != 2 || stdout != "" {
		t.Fatalf(
			"stop without --state-dir = %d with %d bytes of stdout, want 2 and none",
			code, len(stdout),
		)
	}

	if _, err := os.Stat(scene.blobs); err == nil {
		t.Fatalf("the usage error still ran the pipeline: %s exists", scene.blobs)
	}
}

func TestContinueEmitsWhatAFreshRunEmits(t *testing.T) {
	testCases := []struct {
		name   string
		args   func(scene stopScene) []string
		policy string
	}{
		{
			name:   "rest fold",
			args:   func(stopScene) []string { return []string{"--rest", "fold"} },
			policy: "fold",
		},
		{
			name:   "rest wip",
			args:   func(stopScene) []string { return []string{"--rest", "wip"} },
			policy: "wip",
		},
		{
			name:   "one anchor named by a prefix",
			args:   func(scene stopScene) []string { return []string{"--fold", scene.rev[:8]} },
			policy: "fold",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			scene := newStopScene(t)

			if code, _, stderr := runCode(
				t, scene.emit(scene.config(t, "stop"), "--state-dir", scene.state)...,
			); code != 3 {
				t.Fatalf("stop run = %d, want 3\n%s", code, stderr)
			}

			args := append([]string{"continue", "--state", scene.state}, testCase.args(scene)...)

			code, got, stderr := runCode(t, args...)
			if code != 0 {
				t.Fatalf("continue = %d\n%s", code, stderr)
			}

			code, want, stderr := runCode(t, scene.emit(scene.config(t, testCase.policy))...)
			if code != 0 {
				t.Fatalf("fresh run = %d\n%s", code, stderr)
			}

			if got != want {
				t.Fatalf("continue emitted %d bytes, a fresh %s run %d: they differ",
					len(got), testCase.policy, len(want))
			}
		})
	}
}

func TestContinueAbortRemovesTheState(t *testing.T) {
	scene := newStopScene(t)

	if code, _, stderr := runCode(
		t, scene.emit(scene.config(t, "stop"), "--state-dir", scene.state)...,
	); code != 3 {
		t.Fatalf("stop run = %d, want 3\n%s", code, stderr)
	}

	if code, _, stderr := runCode(t, "continue", "--state", scene.state, "--abort"); code != 0 {
		t.Fatalf("abort = %d\n%s", code, stderr)
	}

	if _, err := os.Stat(filepath.Join(scene.state, "state.json")); err == nil {
		t.Fatal("state.json survives --abort")
	}
}

func TestStopThatCannotBeKeptIsNeverAStop(t *testing.T) {
	testCases := []struct {
		name         string
		files        map[string]string
		isDirAFile   bool
		wantSurvivor string
	}{
		{name: "a state directory that is a regular file", isDirAFile: true},
		{
			name:         "a state directory with a state of someone else",
			files:        map[string]string{"state.json": `{"mine":1}`},
			wantSurvivor: "state.json",
		},
		{
			name:         "a state directory with a conflicts directory of someone else",
			files:        map[string]string{"conflicts/data.txt": "tracked"},
			wantSurvivor: "conflicts/data.txt",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			scene := newStopScene(t)

			if testCase.isDirAFile {
				if err := os.WriteFile(scene.state, []byte("in the way"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			for name, text := range testCase.files {
				path := filepath.Join(scene.state, filepath.FromSlash(name))

				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			code, stdout, stderr := runCode(
				t, scene.emit(scene.config(t, "stop"), "--state-dir", scene.state)...,
			)
			if code != 2 || stdout != "" {
				t.Fatalf(
					"stop that cannot be kept = %d with %d bytes of stdout, want 2 and none\n%s",
					code,
					len(stdout),
					stderr,
				)
			}

			if testCase.wantSurvivor == "" {
				return
			}

			got, err := os.ReadFile(
				filepath.Join(scene.state, filepath.FromSlash(testCase.wantSurvivor)),
			)
			if err != nil || string(got) != testCase.files[testCase.wantSurvivor] {
				t.Fatalf("%s = %q, %v, want it untouched", testCase.wantSurvivor, got, err)
			}
		})
	}
}

func TestStopIsSummarisedAsStopped(t *testing.T) {
	testCases := []struct {
		name        string
		verb        string
		extra       []string
		wantCode    int
		wantStopped bool
	}{
		{name: "emit", verb: "emit", wantCode: 3, wantStopped: true},
		{
			name: "emit from the cache", verb: "emit", extra: []string{"--from-cache"},
			wantCode: 3, wantStopped: true,
		},
		{name: "plan", verb: "plan", wantCode: 3, wantStopped: true},
		{
			name: "plan from the cache", verb: "plan", extra: []string{"--from-cache"},
			wantCode: 3, wantStopped: true,
		},
		{
			name: "audit from the cache", verb: "audit", extra: []string{"--from-cache"},
			wantStopped: true,
		},
		{name: "report", verb: "report"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			scene := newStopScene(t)

			var logged bytes.Buffer

			previous := slog.Default()
			slog.SetDefault(
				slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})),
			)
			t.Cleanup(func() { slog.SetDefault(previous) })

			args := slices.Concat([]string{
				testCase.verb, "--repo", scene.source, "--mode", "lineage",
				"--config", scene.config(t, "stop"), "--blob-dir", scene.blobs,
				"--state-dir", scene.state,
			}, testCase.extra)

			code, _, stderr := runCode(t, args...)
			if code != testCase.wantCode {
				t.Fatalf("%s = %d, want %d\n%s", testCase.name, code, testCase.wantCode, stderr)
			}

			if got := strings.Contains(
				logged.String(),
				"outcome=stopped",
			); got != testCase.wantStopped {
				t.Fatalf(
					"summary names the stop = %v, want %v\n%s",
					got,
					testCase.wantStopped,
					logged.String(),
				)
			}

			if strings.Contains(logged.String(), "outcome=folded") {
				t.Fatalf("a stopped anchor is summarised as folded\n%s", logged.String())
			}
		})
	}
}
