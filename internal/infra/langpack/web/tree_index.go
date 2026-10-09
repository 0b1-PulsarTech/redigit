package web

import (
	"context"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"
)

// treeIndex is what Declarations needs from the whole final tree: which
// scripts other files import, and which import one another in a cycle.
// Declarations runs concurrently, so the scan runs once, under the first
// caller's context.
type treeIndex struct {
	space   *workspace
	once    sync.Once
	helpers map[string]bool
	atoms   map[string]string
	err     error
}

func (t *treeIndex) load(ctx context.Context) error {
	t.once.Do(func() {
		var imports map[string][]string

		imports, t.err = t.space.importGraph(ctx)
		t.helpers, t.atoms = importedFromTests(imports), cycleAtoms(imports)
	})

	return t.err
}

// isTest holds for a "*.test.*" or "*.spec.*" file, and for a Node script
// under a test/ directory that no other test file imports: one that is
// imported is a helper.
func (t *treeIndex) isTest(file string) bool {
	base := path.Base(file)
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}

	return isNodeScript(file) && isUnderTest(file) && !t.helpers[file]
}

// importGraph maps every script of the tree to the files of the tree it
// imports.
func (w *workspace) importGraph(ctx context.Context) (map[string][]string, error) {
	out := map[string][]string{}

	for _, file := range slices.Sorted(maps.Keys(w.files)) {
		if !isScript(file) {
			continue
		}

		source, err := w.read(ctx, file)
		if err != nil {
			return nil, err
		}

		out[file] = nil

		for _, spec := range specifiers(string(source)) {
			target, resolveErr := w.resolve(ctx, file, spec)
			if resolveErr != nil {
				return nil, resolveErr
			}

			if target != "" {
				out[file] = append(out[file], target)
			}
		}
	}

	return out, nil
}

func importedFromTests(imports map[string][]string) map[string]bool {
	out := map[string]bool{}

	for file, targets := range imports {
		for _, target := range targets {
			out[target] = out[target] || isUnderTest(file)
		}
	}

	return out
}

func isScript(file string) bool { return slices.Contains(Extensions(), path.Ext(file)) }

func isNodeScript(file string) bool { return path.Ext(file) == mjsExt || path.Ext(file) == jsExt }

func isUnderTest(file string) bool {
	return strings.HasPrefix(file, "test/") || strings.Contains(file, "/test/")
}
