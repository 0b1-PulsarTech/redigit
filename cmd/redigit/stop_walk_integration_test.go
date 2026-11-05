//go:build integration

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
)

type walkScene struct {
	stopScene

	second string
}

func newWalkScene(t *testing.T) walkScene {
	t.Helper()

	scratch := t.TempDir()
	scene := walkScene{stopScene: stopScene{
		source: filepath.Join(scratch, "source"),
		blobs:  filepath.Join(scratch, "blobs"),
		state:  filepath.Join(scratch, "state"),
		cache:  filepath.Join(scratch, "xdg"),
	}}

	if err := os.MkdirAll(scene.source, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	gitText(t, scene.source, "init", "-q", "-b", "main")
	gitText(t, scene.source, "config", "user.email", "tester@example.com")
	gitText(t, scene.source, "config", "user.name", "Tester")

	left := "package aaa\n\nimport \"example.com/p/internal/bbb\"\n\n"
	right := "package ccc\n\nimport \"example.com/p/internal/ddd\"\n\n"
	typeOnly := "package ddd\n\ntype U struct{}\n" + fixPad
	withN := "package ddd\n\ntype U struct{}\n\nfunc (U) N() int { return 1 }\n" + fixPad

	for _, step := range []historyStep{
		{subject: "feat(aaa): add F", files: map[string]string{
			"go.mod":            modFile,
			"internal/bbb/b.go": methodsTypeOnly,
			"internal/ddd/d.go": typeOnly,
			methodsDropFile:     left + "func F(t bbb.T) int { return 1 }\n" + fixPad,
			"internal/ccc/c.go": right + "func G(u ddd.U) int { return 1 }\n" + fixPad,
		}},
		{subject: "feat(bbb): add M", files: map[string]string{
			"internal/bbb/b.go": methodsWithM,
			methodsDropFile:     left + "func F(t bbb.T) int { return t.M() }\n" + fixPad,
		}},
		{subject: "fix(aaa): drop the M call", files: map[string]string{
			methodsDropFile: left + "func F(t bbb.T) int { return 1 }\n" + fixPad,
		}},
		{subject: "feat(ddd): add N", files: map[string]string{
			"internal/ddd/d.go": withN,
			"internal/ccc/c.go": right + "func G(u ddd.U) int { return u.N() }\n" + fixPad,
		}},
		{subject: "fix(ccc): drop the N call", files: map[string]string{
			"internal/ccc/c.go": right + "func G(u ddd.U) int { return 1 }\n" + fixPad,
		}},
		{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
	} {
		commitFiles(t, scene.source, step.subject, step.files)
	}

	scene.rev = gitText(t, scene.source, "log", "--format=%H", "--fixed-strings",
		"--grep=drop the M call", "-1")
	scene.second = gitText(t, scene.source, "log", "--format=%H", "--fixed-strings",
		"--grep=drop the N call", "-1")

	t.Setenv("XDG_CACHE_HOME", scene.cache)

	return scene
}

func (scene walkScene) mix(t *testing.T, first, second string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mix.json")
	anchor := func(rev, policy string) string {
		text := `{"rev": "` + rev + `", "kind": "fix"`
		if policy != "" {
			text += `, "policy": "` + policy + `"`
		}

		return text + "}"
	}
	settings := `{"write": {"identity": {"name": "proof", "email": "proof@localhost"}},` +
		`"lineage": {"policy": "stop", "anchors": [` + anchor(scene.rev, first) + `, ` +
		anchor(scene.second, second) + `]}}`

	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

type sceneStep struct {
	name     string
	args     []string
	before   func()
	after    func()
	wantCode int
	wantLeft int
}

func runSceneStep(t *testing.T, scene walkScene, step sceneStep) string {
	t.Helper()

	if step.before != nil {
		step.before()
	}

	code, stdout, stderr := runCode(t, step.args...)

	if step.after != nil {
		step.after()
	}

	if code != step.wantCode {
		t.Fatalf("%s: exit %d, want %d\n%s", step.name, code, step.wantCode, stderr)
	}

	state, err := anchorstate.Load(scene.state)
	if err != nil || len(state.Conflicts) != step.wantLeft {
		t.Fatalf("%s: state %+v, %v, want %d conflicts", step.name, state, err, step.wantLeft)
	}

	return stdout
}

func TestContinueWalkEmitsWhatAFreshMixEmits(t *testing.T) {
	scene := newWalkScene(t)
	config := scene.mix(t, "", "")

	original, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}

	cont := func(extra ...string) []string {
		return slices.Concat([]string{"continue", "--state", scene.state}, extra)
	}
	steps := []sceneStep{
		{
			name: "the first run stops on both anchors", wantCode: 3, wantLeft: 2,
			args: scene.emit(config, "--state-dir", scene.state),
		},
		{
			name: "a continue that decides one anchor stops again on the other",
			args: cont("--wip", scene.rev[:8]), wantCode: 3, wantLeft: 1,
		},
		{
			name: "an edited config is refused", args: cont("--fold", scene.second[:8]),
			before:   func() { _ = os.WriteFile(config, append(original, '\n'), 0o600) },
			after:    func() { _ = os.WriteFile(config, original, 0o600) },
			wantCode: 1, wantLeft: 1,
		},
		{
			name: "a continue that fails leaves the state as it was",
			args: cont("--fold", scene.second[:8]), wantCode: 1, wantLeft: 1,
			before: func() {
				_ = os.RemoveAll(scene.blobs)
				_ = os.WriteFile(scene.blobs, []byte("in the way"), 0o600)
			},
			after: func() { _ = os.Remove(scene.blobs) },
		},
		{
			name: "the same continue finishes", args: cont("--fold", scene.second[:8]),
			wantCode: 0, wantLeft: 1,
		},
		{
			name: "and finishes again when it is repeated", args: cont("--fold", scene.second[:8]),
			wantCode: 0, wantLeft: 1,
		},
	}

	var finished []string

	for _, step := range steps {
		if stdout := runSceneStep(t, scene, step); step.wantCode == 0 {
			finished = append(finished, stdout)
		}
	}

	code, want, stderr := runCode(
		t,
		scene.emit(
			scene.mix(t, "wip", "fold"),
			"--state-dir",
			filepath.Join(t.TempDir(), "fresh"),
		)...,
	)
	if code != 0 {
		t.Fatalf("fresh run = %d\n%s", code, stderr)
	}

	if len(finished) != 2 || finished[0] != want || finished[1] != want {
		t.Fatalf("the walk emitted %d and %d bytes, a fresh mix %d: they differ",
			len(finished[0]), len(finished[1]), len(want))
	}
}
