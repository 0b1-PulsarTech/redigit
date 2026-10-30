//go:build integration

package plancache

import (
	"path/filepath"
	"testing"
)

func TestGoToolchainSeesAGOENVFile(t *testing.T) {
	dir := t.TempDir()
	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})
	settings := writeFile(
		t, dir, "goenv", "GOOS=windows\nGOFLAGS=-mod=mod\n", 0o600,
	)

	clean, err := GoToolchain(t.Context(), repo, commit)
	if err != nil {
		t.Fatalf("GoToolchain() err = %v", err)
	}

	t.Setenv("GOENV", settings)
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOMODCACHE", filepath.Join(dir, "absent"))

	changed, err := GoToolchain(t.Context(), repo, commit)
	if err != nil {
		t.Fatalf("GoToolchain() err = %v", err)
	}

	if changed == clean {
		t.Fatal("a GOENV file, GOPROXY and GOMODCACHE change replayed the clean key")
	}
}
