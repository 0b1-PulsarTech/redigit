package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type (
	measureOut struct {
		Name     string
		Excluded []string
	}

	summaryOut struct {
		Summary  bool
		Rows     int
		Measured bool
		Note     string
		Findings int
		Measures []measureOut
		Deferred []string
	}
)

func blobDirWith(tb testing.TB, files map[string]string) string {
	tb.Helper()

	dir := tb.TempDir()

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			tb.Fatalf("WriteFile() err = %v", err)
		}
	}

	return dir
}

func manifestLine(path, handle string) string {
	return `{"position":0,"subject":"s0","blobs":[{"path":"` + path +
		`","mode":"100644","handle":"` + handle + `"}]}` + "\n"
}

func manifestFile(tb testing.TB, text string) string {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), "m.jsonl")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		tb.Fatalf("WriteFile() err = %v", err)
	}

	return path
}

func runLint(
	tb testing.TB,
	manifestText string,
	blobDir string,
	extra ...string,
) ([]string, summaryOut, error) {
	tb.Helper()

	args := append([]string{"--manifest", manifestFile(tb, manifestText)}, extra...)
	if blobDir != "" {
		args = append(args, "--blob-dir", blobDir)
	}

	var stdout bytes.Buffer

	runErr := Run(context.Background(), args, &stdout)

	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return nil, summaryOut{}, runErr
	}

	lines := strings.Split(text, "\n")
	got := make([]string, 0, len(lines))

	for _, line := range lines[:len(lines)-1] {
		var finding struct{ Measure string }
		if err := json.Unmarshal([]byte(line), &finding); err != nil {
			tb.Fatalf("finding line %q: %v", line, err)
		}

		got = append(got, finding.Measure)
	}

	var summary summaryOut

	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); err != nil {
		tb.Fatalf("summary line: %v", err)
	}

	return got, summary, runErr
}
