package golang

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// writeFile writes content under root/relPath, creating parent directories.
// Shared by every rule test that needs a real file for its rule to read via
// entities.Snapshot.Root.
func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", relPath, err)
	}

	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

// declareAll runs Syntax.Declarations over every path, relative to root, and
// concatenates the results the way an indexing run would.
func declareAll(t *testing.T, syntax Syntax, root string, paths []string) []entities.Decl {
	t.Helper()

	var all []entities.Decl

	for _, path := range paths {
		source, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		decls, err := syntax.Declarations(context.Background(), path, source)
		if err != nil {
			t.Fatalf("Declarations(%s) error = %v", path, err)
		}

		all = append(all, decls...)
	}

	return all
}
