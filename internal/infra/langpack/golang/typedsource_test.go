package golang

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// typedFixture writes files into a throwaway module and returns the snapshot
// and declarations an indexing run would hold for it.
func typedFixture(t *testing.T, files map[string]string) (entities.Snapshot, []entities.Decl) {
	t.Helper()

	root := t.TempDir()

	writeFile(t, root, "go.mod", "module example.test/fixture\n\ngo 1.26\n")

	snap := entities.Snapshot{Root: root, Files: []string{"go.mod"}}

	goFiles := make([]string, 0, len(files))

	for name, content := range files {
		writeFile(t, root, name, content)

		snap.Files = append(snap.Files, name)
		goFiles = append(goFiles, name)
	}

	sort.Strings(goFiles)

	return snap, declareAll(t, NewSyntax(), root, goFiles)
}

func declKey(file string, kind entities.DeclKind, name string) entities.DeclKey {
	return entities.DeclKey{File: filepath.ToSlash(file), Kind: kind, Name: name}
}

func edgeSet(edges []entities.Edge) map[entities.Edge]bool {
	out := make(map[entities.Edge]bool, len(edges))
	for _, edge := range edges {
		out[edge] = true
	}

	return out
}
