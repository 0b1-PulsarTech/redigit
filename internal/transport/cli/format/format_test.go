package format

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fakeConfig = `{"format":{"languages":[{"name":"go","extensions":[".go"],"steps":[` +
	`{"cmd":["fakefmt","-w"],"mode":"files","timeout":"30s"}]}]}}`

type fixture struct {
	manifest string
	out      string
	blobs    string
	outBlobs string
	config   string
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "redigit-fakefmt-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	build := exec.CommandContext(
		context.Background(), "go", "build", "-o", filepath.Join(dir, "fakefmt"),
		"../../../infra/format/testdata/fakefmt",
	)
	build.Stderr = os.Stderr

	if err = build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build fakefmt:", err)
		os.RemoveAll(dir)
		os.Exit(2)
	}

	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	code := m.Run()

	os.RemoveAll(dir)
	os.Exit(code)
}

func sumOf(content string) string {
	digest := sha256.Sum256([]byte(content))

	return hex.EncodeToString(digest[:])
}

func sumLower() string { return sumOf("LOWER") }

func mustWrite(t *testing.T, name, content string) {
	t.Helper()

	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func stage(t testing.TB, rows []string, blobs map[string]string, config string) fixture {
	t.Helper()

	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	files := map[string]string{
		filepath.Join(dir, "m.jsonl"):       strings.Join(rows, "\n") + "\n",
		filepath.Join(root, "fake.cfg"):     "x",
		filepath.Join(dir, "redigit.json"):  strings.ReplaceAll(config, "{root}", root),
		filepath.Join(dir, "blobs", "keep"): "",
	}

	for handle, content := range blobs {
		files[filepath.Join(dir, "blobs", handle)] = content
	}

	for name, content := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return fixture{
		manifest: filepath.Join(dir, "m.jsonl"), out: filepath.Join(dir, "m2.jsonl"),
		blobs: filepath.Join(dir, "blobs"), outBlobs: filepath.Join(dir, "blobs2"),
		config: filepath.Join(dir, "redigit.json"),
	}
}

func (fix fixture) args(extra ...string) []string {
	return append([]string{
		"--manifest", fix.manifest, "--out", fix.out, "--blob-dir", fix.blobs,
		"--out-blob-dir", fix.outBlobs, "--config", fix.config,
	}, extra...)
}
