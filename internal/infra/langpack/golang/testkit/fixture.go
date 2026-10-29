package testkit

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

func DeclareAll(t *testing.T, root string, paths []string) []entities.Decl {
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

func Fixture(t *testing.T, files map[string]string) (entities.Snapshot, []entities.Decl) {
	t.Helper()

	root := t.TempDir()

	WriteFile(t, root, "go.mod", "module example.test/fixture\n\ngo 1.26\n")

	snap := entities.Snapshot{Root: root, Files: []string{"go.mod"}}

	goFiles := make([]string, 0, len(files))

	for name, content := range files {
		WriteFile(t, root, name, content)

		snap.Files = append(snap.Files, name)
		goFiles = append(goFiles, name)
	}

	sort.Strings(goFiles)

	return snap, DeclareAll(t, root, goFiles)
}

func DeclKey(file string, kind entities.DeclKind, name string) entities.DeclKey {
	return entities.DeclKey{File: filepath.ToSlash(file), Kind: kind, Name: name}
}

func EdgeSet(edges []entities.Edge) map[entities.Edge]bool {
	out := make(map[entities.Edge]bool, len(edges))
	for _, edge := range edges {
		out[edge] = true
	}

	return out
}
