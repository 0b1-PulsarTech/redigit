//go:build integration

package cli

import (
	"bytes"
	"cmp"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type verifyRow struct {
	Status  string `json:"status"`
	Wip     bool   `json:"wip"`
	Summary bool   `json:"summary"`
	Failed  int    `json:"failed"`
}

func gitIn(t *testing.T, dir string, args ...string) {
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

func commitAtk(t *testing.T, repo, routes string) {
	t.Helper()

	stages := [][]string{
		{"go.mod", "router/router.go"},
		{"users/users.go"},
		{"orders/orders.go"},
		{"billing/billing.go"},
		{"app/routes.go", "cmd/srv/main.go"},
	}

	fixtures := filepath.Join("..", "..", "infra", "langpack", "idiom", "testdata")

	for _, stage := range stages {
		for _, name := range stage {
			source := filepath.Join(fixtures, "atk", name)
			if name == "app/routes.go" && routes != "" {
				source = filepath.Join(fixtures, routes)
			}

			body, err := os.ReadFile(source)
			if err != nil {
				t.Fatalf("read %s: %v", source, err)
			}

			target := filepath.Join(repo, name)
			if err = os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			if err = os.WriteFile(target, body, 0o600); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}

		gitIn(t, repo, "add", "-A")
		gitIn(t, repo, "commit", "-q", "-m", "feat: "+stage[0])
	}
}

func runStage(t *testing.T, args ...string) string {
	t.Helper()

	var stdout, stderr bytes.Buffer
	if err := Run(t.Context(), args, &stdout, &stderr); err != nil {
		t.Fatalf("redigit %v: %v\n%s\n%s", args, err, stdout.String(), stderr.String())
	}

	return stdout.String()
}

func requirePassingRows(t *testing.T, out string) {
	t.Helper()

	rows := 0

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		var row verifyRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("row %q: %v", line, err)
		}

		if row.Summary {
			if row.Failed != 0 {
				t.Fatalf("summary reports %d failing rows:\n%s", row.Failed, out)
			}

			continue
		}

		rows++

		if row.Status != "pass" {
			t.Fatalf("a rewritten commit does not build:\n%s", out)
		}
	}

	if rows == 0 {
		t.Fatalf("verify checked no commit:\n%s", out)
	}
}

func TestRewrittenHistoryOfAMountingModuleBuildsOnEveryCommit(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "")

	testCases := []struct {
		name   string
		routes string
		config string
	}{
		{name: "three mounts in one statement"},
		{name: "one mount per statement", routes: "atk_routes_per_statement.go.txt"},
		{name: "mounts in statements of one resource", routes: "atk_routes_one_resource.go.txt"},
		{name: "two rules with a caller in common", config: "atk_two_rules_config.json"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			scratch := t.TempDir()
			repo := filepath.Join(scratch, "repo")
			config := filepath.Join("testdata", cmp.Or(testCase.config, "atk_config.json"))
			manifest := filepath.Join(scratch, "emit.json")

			gitIn(t, scratch, "init", "-q", "-b", "main", repo)
			commitAtk(t, repo, testCase.routes)

			base := []string{"--repo", repo, "--config", config}

			if plan := runStage(t, append([]string{"plan"}, base...)...); plan == "" {
				t.Fatalf("plan printed nothing")
			}

			blobs := filepath.Join(scratch, "blobs")
			emitted := runStage(t, append([]string{"emit", "--blob-dir", blobs}, base...)...)

			if err := os.WriteFile(manifest, []byte(emitted), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			runStage(
				t,
				"write",
				"--repo",
				repo,
				"--manifest",
				manifest,
				"--ref",
				"refs/heads/proof",
				"--date-ref",
				"HEAD",
				"--verify",
				"HEAD",
				"--config",
				config,
				"--allow-draft",
				"--blob-dir",
				blobs,
			)

			out := runStage(t, "verify", "--repo", repo, "--ref", "refs/heads/proof",
				"--cache-dir", filepath.Join(scratch, "cache"))

			requirePassingRows(t, out)
		})
	}
}
