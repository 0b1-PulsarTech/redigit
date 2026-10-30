//go:build integration

package verify

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const goMain = "package main\n\nfunc main() { println(value()) }\n\nfunc value() int { return 1 }\n"

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitMain(t *testing.T, dir, subject, source string) {
	t.Helper()

	files := map[string]string{
		"go.mod":  "module example.com/probe\n\ngo 1.21\n",
		"main.go": source,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", subject)
}

func statuses(t *testing.T, stdout string) ([]string, map[string]any) {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	got := make([]string, 0, len(lines))

	for _, line := range lines[:len(lines)-1] {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("row %q: %v", line, err)
		}

		status, _ := row["status"].(string)
		if cached, _ := row["cached"].(bool); cached {
			status += " cached"
		}

		got = append(got, status)

		if row["status"] == "fail" && !strings.Contains(row["tail"].(string), "undefined") {
			t.Fatalf("a failing row must carry the compiler output, got %v", row)
		}
	}

	var summary map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); err != nil {
		t.Fatalf("summary %q: %v", lines[len(lines)-1], err)
	}

	return got, summary
}

func TestRunBuildsEveryCommitWithTheRealToolchain(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "")

	repo := t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")
	commitMain(t, repo, "feat: add the program", goMain)
	broken := strings.Replace(goMain, "println(value())", "println(missing())", 1)
	commitMain(t, repo, "fix: call a helper", broken)
	commitMain(t, repo, "fix: define the helper", goMain)

	cache := filepath.Join(t.TempDir(), "cache")
	args := []string{"--repo", repo, "--cache-dir", cache, "--parallel", "1"}

	testCases := []struct {
		name string
		want []string
	}{
		{
			"a cold run reuses the green tree it already built",
			[]string{"pass", "fail", "pass cached"},
		},
		{"a warm run rebuilds only the red commit", []string{"pass cached", "fail", "pass cached"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Run(t.Context(), args, &out); !errors.Is(err, ErrFailed) {
				t.Fatalf("Run() err = %v, want ErrFailed for the red commit", err)
			}

			got, summary := statuses(t, out.String())
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("rows = %v, want %v", got, testCase.want)
			}

			if summary["ok"] != false || summary["failed"] != float64(1) ||
				summary["passed"] != float64(2) {
				t.Fatalf("summary = %v, want one failure and two passes", summary)
			}

			assertSlotsRemoved(t, filepath.Join(cache, "slots"))
		})
	}
}

func TestRunDoesNotReplayAPassWhenOnlyALinkTargetChanged(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "")

	repo := t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")

	for _, dir := range []string{"moda", "shared"} {
		if err := os.MkdirAll(filepath.Join(repo, dir), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	files := map[string]string{
		"moda/go.mod":       "module example.com/moda\n\ngo 1.21\n",
		"moda/a.go":         "package moda\n",
		"shared/shared.txt": "package moda\n\nfunc Shared() int { return 1 }\n",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := os.Symlink(
		"../shared/shared.txt",
		filepath.Join(repo, "moda/shared.go"),
	); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "feat: link the shared source")

	broken := "package moda\n\nvar broken = undefinedThing\n"
	if err := os.WriteFile(
		filepath.Join(repo, "shared/shared.txt"),
		[]byte(broken),
		0o600,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	gitRun(t, repo, "commit", "-q", "-am", "fix: change only the link target")

	var out bytes.Buffer

	args := []string{
		"--repo",
		repo,
		"--cache-dir",
		filepath.Join(t.TempDir(), "c"),
		"--parallel",
		"1",
	}
	if err := Run(t.Context(), args, &out); !errors.Is(err, ErrFailed) {
		t.Fatalf("Run() err = %v, want ErrFailed for the broken link target", err)
	}

	got, _ := statuses(t, out.String())
	if strings.Join(got, ",") != "pass,fail" {
		t.Fatalf("rows = %v, want a fresh fail, never a replayed pass", got)
	}
}

func writeFiles(t *testing.T, repo string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}

		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestRunDoesNotReplayAPassWhenOnlyTheWorkspaceVendorChanged(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "")

	repo := t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")
	writeFiles(t, repo, map[string]string{
		"go.work":          "go 1.22\n\nuse ./app\n\nreplace example.com/dep => ./_deps/dep\n",
		"app/go.mod":       "module example.com/app\n\ngo 1.22\n\nrequire example.com/dep v0.0.0\n",
		"app/main.go":      "package main\n\nimport \"example.com/dep\"\n\nfunc main() { println(dep.Value()) }\n",
		"_deps/dep/go.mod": "module example.com/dep\n\ngo 1.22\n",
		"_deps/dep/dep.go": "package dep\n\nfunc Value() int { return 1 }\n",
	})

	vendor := exec.CommandContext(t.Context(), "go", "work", "vendor")
	vendor.Dir = repo

	if out, err := vendor.CombinedOutput(); err != nil {
		t.Fatalf("go work vendor: %v\n%s", err, out)
	}

	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "feat: vendor the workspace")

	writeFiles(t, repo, map[string]string{
		"vendor/example.com/dep/dep.go": "package dep\n\nvar broken = undefinedThing\n",
	})
	gitRun(t, repo, "commit", "-q", "-am", "fix: change only the vendor tree")

	var out bytes.Buffer

	args := []string{
		"--repo", repo, "--cache-dir", filepath.Join(t.TempDir(), "c"), "--parallel", "1",
	}
	if err := Run(t.Context(), args, &out); !errors.Is(err, ErrFailed) {
		t.Fatalf("Run() err = %v, want ErrFailed for the broken vendor tree", err)
	}

	got, _ := statuses(t, out.String())
	if strings.Join(got, ",") != "pass,fail" {
		t.Fatalf("rows = %v, want a fresh fail, never a replayed pass", got)
	}
}

func TestRunDoesNotReplayAPassWhenOnlyTheCIncludePathChanged(t *testing.T) {
	const cgoMain = "package probe\n\n/*\n#include <probe.h>\n*/\nimport \"C\"\n\nvar Value = C.PROBE\n"

	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler")
	}

	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "")
	t.Setenv("CGO_ENABLED", "1")
	t.Setenv("CC", "gcc")

	headers, repo, caches := t.TempDir(), t.TempDir(), t.TempDir()
	writeFiles(t, headers, map[string]string{
		"hA/probe.h": "#define PROBE 1\n",
		"hB/probe.h": "#error undefined probe header\n",
	})
	gitRun(t, repo, "init", "-q", "-b", "main")
	commitMain(t, repo, "feat: include a header", cgoMain)

	args := []string{
		"--repo", repo, "--cache-dir", filepath.Join(t.TempDir(), "c"), "--parallel", "1",
	}
	testCases := []struct {
		name    string
		include string
		want    string
		wantErr error
	}{
		{"the first header passes", "hA", "pass", nil},
		{"the same header replays the pass", "hA", "pass cached", nil},
		{"another header is rebuilt and fails", "hB", "fail", ErrFailed},
	}

	t.Setenv("GOCACHE", caches)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("CPATH", filepath.Join(headers, testCase.include))

			var out bytes.Buffer

			if err := Run(t.Context(), args, &out); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Run() err = %v, want %v", err, testCase.wantErr)
			}

			if got, _ := statuses(t, out.String()); strings.Join(got, ",") != testCase.want {
				t.Fatalf("rows = %v, want %v", got, testCase.want)
			}
		})
	}
}

func assertSlotsRemoved(t *testing.T, root string) {
	t.Helper()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read slots dir: %v", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".lock") {
			t.Fatalf("%s survived the run, want only lock files", entry.Name())
		}
	}
}
