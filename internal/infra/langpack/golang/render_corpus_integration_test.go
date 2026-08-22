//go:build integration

package golang

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// TestRenderRoundTripsRealSource is the validation the renderer cannot be
// trusted without: rendering a file while keeping EVERY declaration must
// reproduce it byte for byte. A fixture corpus proves the cases somebody
// thought of; this walks real code, including the shapes nobody enumerated -
// build constraints, generate directives, blank-identifier assertions, grouped
// consts with iota, generics, and external test packages.
func TestRenderRoundTripsRealSource(t *testing.T) {
	root := repoRoot(t)
	syntax := NewSyntax()
	ctx := context.Background()

	var checked int

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if name := entry.Name(); name == ".git" || name == "testdata" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		decls, err := syntax.Declarations(ctx, rel, source)
		if err != nil {
			t.Errorf("%s: Declarations() = %v", rel, err)

			return nil
		}

		keys := make([]entities.DeclKey, 0, len(decls))
		for _, decl := range decls {
			keys = append(keys, decl.Key)
		}

		rendered, err := syntax.Render(ctx, source, keys)
		if err != nil {
			t.Errorf("%s: Render() = %v", rel, err)

			return nil
		}

		if string(rendered) != string(source) {
			t.Errorf("%s: keeping every declaration did not reproduce the file", rel)
		}

		checked++

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked < 20 {
		t.Fatalf("only %d files checked, which is too few to prove anything", checked)
	}

	t.Logf("round-tripped %d real files", checked)
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for range 8 {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}

		dir = filepath.Dir(dir)
	}

	t.Fatal("no go.mod above the test directory")

	return ""
}
