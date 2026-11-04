//go:build integration

package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli"
)

type historyStep struct {
	subject string
	files   map[string]string
	remove  []string
}

type shapeWant struct {
	kept, absent, reasons, without []string
	marked                         int
}

type anchoredRun struct {
	stderr     string
	subjects   []string
	isTreeSame bool
}

func runCaptured(t *testing.T, args ...string) (stdoutText, stderrText string) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	if code := run(context.WithoutCancel(t.Context()), args, &stdout, &stderr, cli.Run); code != 0 {
		t.Fatalf("redigit %v = %d\n%s", args, code, stderr.String())
	}

	return stdout.String(), stderr.String()
}

func runHistory(t *testing.T, steps []historyStep, anchors []string, extra string) anchoredRun {
	t.Helper()

	return runPolicy(t, "fold", steps, anchors, extra)
}

func runPolicy(
	t *testing.T,
	policy string,
	steps []historyStep,
	anchors []string,
	extra string,
) anchoredRun {
	t.Helper()

	scratch := t.TempDir()
	source := filepath.Join(scratch, "source")
	bare := filepath.Join(scratch, "bare")
	blobs := filepath.Join(scratch, "blobs")

	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	gitText(t, source, "init", "-q", "-b", "main")
	gitText(t, source, "config", "user.email", "tester@example.com")
	gitText(t, source, "config", "user.name", "Tester")

	for _, step := range steps {
		for _, name := range step.remove {
			if err := os.Remove(filepath.Join(source, name)); err != nil {
				t.Fatalf("remove %s: %v", name, err)
			}
		}

		commitFiles(t, source, step.subject, step.files)
	}

	specs := make([]map[string]string, 0, len(anchors))

	for _, subject := range anchors {
		rev := gitText(t, source, "log", "--format=%H", "--fixed-strings", "--grep="+subject, "-1")
		specs = append(specs, map[string]string{"rev": rev, "kind": "fix"})
	}

	encoded, err := json.Marshal(specs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	settings := `{"write": {"identity": {"name": "proof", "email": "proof@localhost"}},` +
		`"lineage": {"policy": "` + policy + `", "anchors": ` + string(encoded) + `}` + extra + `}`
	config := filepath.Join(scratch, "cfg.json")

	if err = os.WriteFile(config, []byte(settings), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	gitText(t, scratch, "clone", "-q", "--bare", "--no-local", source, bare)

	var logged bytes.Buffer

	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	emitted, stderr := runCaptured(t, "emit", "--repo", source, "--mode", "lineage",
		"--config", config, "--blob-dir", blobs)
	manifest := filepath.Join(scratch, "emit.json")

	if err = os.WriteFile(manifest, []byte(emitted), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	runCLI(t, "write", "--repo", bare, "--manifest", manifest, "--ref", "refs/heads/proof",
		"--date-ref", "HEAD", "--verify", "HEAD", "--config", config, "--allow-draft",
		"--blob-dir", blobs)
	runCLI(t, "verify", "--repo", bare, "--ref", "refs/heads/proof",
		"--cache-dir", filepath.Join(scratch, "verify"), "--config", config)

	written := gitText(t, bare, "rev-parse", "proof^{tree}")

	return anchoredRun{
		stderr:     stderr + logged.String(),
		subjects:   strings.Split(gitText(t, bare, "log", "--format=%s", "refs/heads/proof"), "\n"),
		isTreeSame: written == gitText(t, bare, "rev-parse", "HEAD^{tree}"),
	}
}

const modFile = "module example.com/p\n\ngo 1.21\n"

type historyCase struct {
	name    string
	policy  string
	steps   []historyStep
	anchors []string
	extra   string
	want    shapeWant
}

func assertShape(t *testing.T, got anchoredRun, want shapeWant) {
	t.Helper()

	if !got.isTreeSame {
		t.Fatalf("the written tree differs from the source tree")
	}

	for _, subject := range want.kept {
		if !slices.Contains(got.subjects, subject) {
			t.Fatalf("subject %q missing from %q", subject, got.subjects)
		}
	}

	for _, subject := range want.absent {
		if slices.Contains(got.subjects, subject) {
			t.Fatalf("subject %q should be folded, log %q", subject, got.subjects)
		}
	}

	assertReasons(t, got, want)
}

func assertReasons(t *testing.T, got anchoredRun, want shapeWant) {
	t.Helper()

	for _, reason := range want.reasons {
		if !strings.Contains(got.stderr, reason) {
			t.Fatalf("reason %q missing from the report:\n%s", reason, got.stderr)
		}
	}

	for _, reason := range want.without {
		if strings.Contains(got.stderr, reason) {
			t.Fatalf("text %q must not appear in the report:\n%s", reason, got.stderr)
		}
	}

	marked := 0

	for _, subject := range got.subjects {
		if strings.Contains(subject, "[wip]") {
			marked++
		}
	}

	if (want.marked > 0) != (marked > 0) {
		t.Fatalf(
			"marked commits = %d, want some = %v, log %q",
			marked,
			want.marked > 0,
			got.subjects,
		)
	}
}

func assertCases(t *testing.T, testCases []historyCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runPolicy(t, cmp.Or(testCase.policy, "fold"),
				testCase.steps, testCase.anchors, testCase.extra)
			assertShape(t, got, testCase.want)
		})
	}
}
