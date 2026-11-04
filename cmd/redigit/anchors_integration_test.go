//go:build integration

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli"
)

type anchoredScene struct {
	source string
	bare   string
	config string
	blobs  string
	fixRev string
}

func gitText(t testing.TB, dir string, args ...string) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...).
		CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

func commitFiles(t testing.TB, dir, subject string, files map[string]string) {
	t.Helper()

	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	gitText(t, dir, "add", "-A")
	gitText(t, dir, "commit", "-q", "-m", subject)
}

func newAnchoredScene(t *testing.T) anchoredScene {
	t.Helper()

	scratch := t.TempDir()
	scene := anchoredScene{
		source: filepath.Join(scratch, "source"),
		bare:   filepath.Join(scratch, "bare"),
		config: filepath.Join(scratch, "cfg.json"),
		blobs:  filepath.Join(scratch, "blobs"),
	}

	if err := os.MkdirAll(scene.source, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	gitText(t, scene.source, "init", "-q", "-b", "main")
	gitText(t, scene.source, "config", "user.email", "tester@example.com")
	gitText(t, scene.source, "config", "user.name", "Tester")

	commitFiles(t, scene.source, "feat(p): add A and B", map[string]string{
		"go.mod": "module example.com/p\n\ngo 1.21\n",
		"a.go":   "package p\n\nfunc A() int { return 1 }\n",
		"b.go":   "package p\n\nfunc B() int { return A() }\n",
	})
	commitFiles(t, scene.source, "fix(p): repair A", map[string]string{
		"a.go": "package p\n\nfunc A() int { return 2 }\n",
	})

	scene.fixRev = gitText(t, scene.source, "rev-parse", "HEAD")

	commitFiles(t, scene.source, "feat(p): grow A and add C", map[string]string{
		"a.go": "package p\n\nfunc A() int { return 3 }\n",
		"c.go": "package p\n\nfunc C() int { return A() }\n",
	})
	gitText(t, scratch, "clone", "-q", "--bare", "--no-local", scene.source, scene.bare)

	settings := `{"write": {"identity": {"name": "proof", "email": "proof@localhost"}},
		"lineage": {"anchors": [{"rev": "` + scene.fixRev + `", "kind": "fix"}]}}`
	if err := os.WriteFile(scene.config, []byte(settings), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return scene
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()

	var stdout, stderr bytes.Buffer

	if code := run(context.WithoutCancel(t.Context()), args, &stdout, &stderr, cli.Run); code != 0 {
		t.Fatalf("redigit %v = %d\n%s", args, code, stderr.String())
	}

	return stdout.String()
}

func TestAnchoredLineageKeepsTheFixAndTheTree(t *testing.T) {
	testCases := []struct {
		name        string
		fixSubject  string
		wantTouched string
	}{
		{
			name:        "the fix is kept once and touches its file",
			fixSubject:  "fix(p): repair A",
			wantTouched: "a.go",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			scene := newAnchoredScene(t)
			manifest := filepath.Join(t.TempDir(), "emit.json")
			emitted := runCLI(t, "emit", "--repo", scene.source, "--mode", "lineage",
				"--config", scene.config, "--blob-dir", scene.blobs)

			if err := os.WriteFile(manifest, []byte(emitted), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			runCLI(
				t,
				"write",
				"--repo",
				scene.bare,
				"--manifest",
				manifest,
				"--ref",
				"refs/heads/proof",
				"--date-ref",
				"HEAD",
				"--verify",
				"HEAD",
				"--config",
				scene.config,
				"--allow-draft",
				"--blob-dir",
				scene.blobs,
			)

			if got, want := gitText(t, scene.bare, "rev-parse", "proof^{tree}"),
				gitText(t, scene.bare, "rev-parse", "HEAD^{tree}"); got != want {
				t.Fatalf("tree of the written ref = %s, want %s", got, want)
			}

			fixes := gitText(t, scene.bare, "log", "--format=%H", "--fixed-strings",
				"--grep="+testCase.fixSubject, "refs/heads/proof")
			if len(strings.Fields(fixes)) != 1 {
				t.Fatalf("the fix commit appears %q times, want once", fixes)
			}

			touched := gitText(
				t,
				scene.bare,
				"diff-tree",
				"--no-commit-id",
				"--name-only",
				"-r",
				fixes,
			)
			if touched != testCase.wantTouched {
				t.Fatalf("the fix commit changes %q, want %q", touched, testCase.wantTouched)
			}

			runCLI(t, "verify", "--repo", scene.bare, "--ref", "refs/heads/proof",
				"--cache-dir", filepath.Join(t.TempDir(), "verify"))
		})
	}
}
