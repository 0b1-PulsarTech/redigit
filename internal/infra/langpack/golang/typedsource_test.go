package golang

import (
	"context"
	"path/filepath"
	"slices"
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

// A function cut into statements is a different set of declarations, and a use
// belongs to the statement that writes it. Answering from the first set would
// leave every use on the function that no longer holds a body.
func TestTypedSourceAttributesUsesToTheDeclarationsAsked(t *testing.T) {
	t.Parallel()

	snap, decls := typedFixture(t, map[string]string{
		"a.go": "package fixture\n\nfunc One() {}\n\nfunc Run() {\n\tOne()\n}\n",
	})

	source := newTypedSource(nil)

	whole, err := source.Facts(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	run := declKey("a.go", entities.KindFunc, "Run")
	statement := declKey("a.go", entities.KindStmt, "Run#0")
	one := declKey("a.go", entities.KindFunc, "One")

	if !slices.Contains(whole.Uses, use{Dependent: run, Provider: one}) {
		t.Fatalf("Uses = %v, want Run to use One", whole.Uses)
	}

	cut := slices.Clone(decls)

	for index, decl := range cut {
		if decl.Key != run {
			continue
		}

		head := decl
		head.Span.End = head.Span.Start + len("func Run() {")
		body := entities.Decl{
			Key:  statement,
			Unit: decl.Unit,
			Span: entities.Span{Start: head.Span.End + 2, End: decl.Span.End - 2},
		}
		cut[index] = head
		cut = append(cut, body)

		break
	}

	got, err := source.Facts(context.Background(), snap, cut)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	if !slices.Contains(got.Uses, use{Dependent: statement, Provider: one}) ||
		slices.Contains(got.Uses, use{Dependent: run, Provider: one}) {
		t.Fatalf("Uses = %v, want the statement to use One and the head not", got.Uses)
	}
}
