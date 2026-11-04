//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli"
)

type graphEdge struct {
	Dependent string `json:"dependent"`
	Provider  string `json:"provider"`
	Reason    string `json:"reason"`
}

func TestErrorsAsTargetReachesErrorBeforeTheReaderIsWritten(t *testing.T) {
	const (
		pointerError = "package zzz\n\nfunc (*UnhandledError) Error() string { return \"u\" }\n"
		valueError   = "package zzz\n\nfunc (UnhandledError) Error() string { return \"u\" }\n"
	)

	wantEdge := graphEdge{
		Dependent: "aaa/handler.go:func:WrapEmit",
		Provider:  "zzz/error.go:method:UnhandledError.Error",
		Reason:    "implements",
	}

	testCases := []struct {
		name       string
		errorFile  string
		targetType string
	}{
		{
			name:       "a pointer variable, a pointer-receiver Error",
			errorFile:  pointerError,
			targetType: "*zzz.UnhandledError",
		},
		{
			name:       "a pointer variable, a value-receiver Error",
			errorFile:  valueError,
			targetType: "*zzz.UnhandledError",
		},
		{
			name:       "a value variable, a value-receiver Error",
			errorFile:  valueError,
			targetType: "zzz.UnhandledError",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			scratch := t.TempDir()
			source := newErrorsAsSource(t, filepath.Join(scratch, "source"), testCase.errorFile,
				testCase.targetType)
			bare := filepath.Join(scratch, "bare")

			gitText(t, scratch, "clone", "-q", "--bare", "--no-local", source, bare)

			var graph struct {
				Edges []graphEdge `json:"edges"`
			}

			graphText := runCLI(t, "graph", "--repo", source)

			if err := json.Unmarshal([]byte(graphText), &graph); err != nil {
				t.Fatalf("decode graph: %v", err)
			}

			if !slices.Contains(graph.Edges, wantEdge) {
				t.Fatalf("graph holds no edge %+v", wantEdge)
			}

			writeAndVerify(t, scratch, source, bare)
		})
	}
}

func newErrorsAsSource(t *testing.T, source, errorFile, targetType string) string {
	t.Helper()

	reader := fmt.Sprintf("package aaa\n\nimport (\n\t\"errors\"\n\n\t\"example.com/p/zzz\"\n)\n\n"+
		"func WrapEmit(err error) bool {\n\tvar target %s\n\n\treturn errors.As(err, &target)\n}\n",
		targetType)
	filler := map[string]string{"zzz/error.go": errorFile}

	for count := range 14 {
		filler[fmt.Sprintf("zzz/f%02d.go", count)] = fmt.Sprintf(
			"package zzz\n\nfunc Fill%02d() int { return %d }\n", count, count+1)
	}

	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	gitText(t, source, "init", "-q", "-b", "main")
	gitText(t, source, "config", "user.email", "tester@example.com")
	gitText(t, source, "config", "user.name", "Tester")
	writeTree(t, source, map[string]string{
		"go.mod":         "module example.com/p\n\ngo 1.21\n",
		"aaa/handler.go": reader,
		"zzz/type.go":    "package zzz\n\ntype UnhandledError struct{}\n",
	})
	gitText(t, source, "add", "-A")
	gitText(t, source, "commit", "-q", "-m", "feat: the reader and the type")
	writeTree(t, source, filler)
	gitText(t, source, "add", "-A")
	gitText(t, source, "commit", "-q", "-m", "feat: the Error method")

	return source
}

func writeAndVerify(t *testing.T, scratch, source, bare string) {
	t.Helper()

	config := filepath.Join(scratch, "cfg.json")
	blobs := filepath.Join(scratch, "blobs")
	manifest := filepath.Join(scratch, "emit.json")
	identity := `{"write":{"identity":{"name":"proof","email":"proof@localhost"}}}`

	if err := os.WriteFile(config, []byte(identity), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	emitted := runCLI(t, "emit", "--repo", source, "--blob-dir", blobs)
	if err := os.WriteFile(manifest, []byte(emitted), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	runCLI(t, "write", "--repo", bare, "--manifest", manifest, "--ref", "refs/heads/proof",
		"--date-ref", "HEAD", "--verify", "HEAD", "--config", config, "--allow-draft",
		"--blob-dir", blobs)

	if count := gitText(t, bare, "rev-list", "--count", "refs/heads/proof"); count == "1" {
		t.Fatalf("the plan holds %s commit, want the reader and the method apart", count)
	}

	runCLI(t, "verify", "--repo", bare, "--ref", "refs/heads/proof",
		"--cache-dir", filepath.Join(scratch, "verify"))
}

func writeTree(t testing.TB, dir string, files map[string]string) {
	t.Helper()

	for name, text := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}

		if err := os.WriteFile(full, []byte(text), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestVerifyBuildsAModuleWhoseOnlyPackageIsMainWithTests(t *testing.T) {
	const testHead = "package main\n\nimport \"testing\"\n\n"

	testCases := []struct {
		name       string
		testFile   string
		wantFailed bool
		wantStderr string
	}{
		{
			name:     "a passing test file passes",
			testFile: testHead + "func TestOk(t *testing.T) {}\n",
		},
		{
			name:       "a compile error in the test file still fails at go vet",
			testFile:   testHead + "func TestBad(t *testing.T) { var count int = \"text\" }\n",
			wantFailed: true,
			wantStderr: "go vet",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source")

			if err := os.MkdirAll(source, 0o750); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			gitText(t, source, "init", "-q", "-b", "main")
			gitText(t, source, "config", "user.email", "tester@example.com")
			gitText(t, source, "config", "user.name", "Tester")
			commitFiles(t, source, "test: a module with a test and no code", map[string]string{
				"go.mod":       "module example.com/p\n\ngo 1.21\n",
				"main_test.go": testCase.testFile,
			})

			var stdout, stderr bytes.Buffer

			code := run(context.WithoutCancel(t.Context()), []string{
				"verify", "--repo", source, "--ref", "refs/heads/main",
				"--cache-dir", t.TempDir(),
			}, &stdout, &stderr, cli.Run)

			if (code != 0) != testCase.wantFailed {
				t.Fatalf("verify = %d, want failed = %v\n%s\n%s",
					code, testCase.wantFailed, stdout.String(), stderr.String())
			}

			if !strings.Contains(stdout.String()+stderr.String(), testCase.wantStderr) {
				t.Fatalf("verify output lacks %q\n%s\n%s",
					testCase.wantStderr, stdout.String(), stderr.String())
			}
		})
	}
}
