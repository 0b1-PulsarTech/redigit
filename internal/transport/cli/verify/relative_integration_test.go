//go:build integration

package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunKeepsEveryArtifactInsideARelativeCacheDir(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "")

	testCases := []struct {
		name     string
		cacheDir string
		wantTop  string
	}{
		{"a bare name", "rel", "rel"},
		{"a dotted path", filepath.Join(".", "sub", "rel"), "sub"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := t.TempDir()
			gitRun(t, repo, "init", "-q", "-b", "main")
			commitMain(t, repo, "feat: add the program", goMain)

			cwd := t.TempDir()
			t.Chdir(cwd)

			var out bytes.Buffer

			args := []string{"--repo", repo, "--cache-dir", testCase.cacheDir, "--parallel", "1"}
			if err := Run(t.Context(), args, &out); err != nil {
				t.Fatalf("Run() err = %v\n%s", err, out.String())
			}

			got, _ := statuses(t, out.String())
			if strings.Join(got, ",") != "pass" {
				t.Fatalf("rows = %v, want a pass\n%s", got, out.String())
			}

			entries, err := os.ReadDir(cwd)
			if err != nil {
				t.Fatalf("read cwd: %v", err)
			}

			if len(entries) != 1 || entries[0].Name() != testCase.wantTop {
				t.Fatalf("cwd holds %v, want only %s", entries, testCase.wantTop)
			}
		})
	}
}
