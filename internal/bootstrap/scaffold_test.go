package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/infra/chunk"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
)

// fixtureCode is a Go declaration of a fixture, with the declarations it reads.
type fixtureCode struct {
	file, name string
	reads      []string
}

func goDecl(file, name string) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit: entities.UnitID(filepath.Dir(file)),
	}
}

// commitOf plans a fixture repository the way an emit does - config rules,
// atoms, ordering, planning - and reports the commit each file is first born
// in. Go source is declared by hand; every other file is chunked for real.
func commitOf(
	t *testing.T,
	files map[string]string,
	code []fixtureCode,
) map[string]entities.Position {
	t.Helper()

	root := t.TempDir()
	snap := entities.Snapshot{Root: root}

	var decls []entities.Decl

	for name, body := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}

		snap.Files = append(snap.Files, name)

		pieces := wholeFiles(t, name, body)
		decls = append(decls, pieces...)
	}

	var edges []entities.Edge

	for _, each := range code {
		decls = append(decls, goDecl(each.file, each.name))

		for _, read := range each.reads {
			edges = append(edges, entities.Edge{
				Dependent: goDecl(each.file, each.name).Key,
				Provider:  goDecl(read, read).Key,
				Reason:    entities.ReasonReference,
			})
		}
	}

	pack := config.NewRulePack()

	found, err := pack.Edges(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	anchors, err := pack.Anchors(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Anchors() = %v", err)
	}

	graph := entities.NewGraph(decls, append(edges, found...), anchors)

	order, err := ordering.New(ordering.Config{}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	plan, err := planning.New(planning.Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	born := map[string]entities.Position{}

	for _, commit := range plan.Commits {
		for _, key := range commit.Decls {
			if _, seen := born[key.File]; !seen {
				born[key.File] = commit.Position
			}
		}
	}

	return born
}

func wholeFiles(t *testing.T, name, body string) []entities.Decl {
	t.Helper()

	for _, chunker := range chunk.Registry {
		if !chunker.Handles(name) {
			continue
		}

		pieces, err := chunker.Chunks(t.Context(), name, []byte(body))
		if err != nil {
			t.Fatalf("chunk %s: %v", name, err)
		}

		return pieces
	}

	t.Fatalf("no chunker claims %s", name)

	return nil
}

func TestScaffoldIsBornBeforeProductCode(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"apps/app/go.mod":     "module example.com/app\n\ngo 1.26\n",
		".gitignore":          "bin/\n",
		".idea/vcs.xml":       "<project/>\n",
		"Taskfile.yml":        "version: '3'\n",
		"tools/.golangci.yml": "version: '2'\n",
		"tools/go.mod":        "module example.com/tools\n\ngo 1.26\n",
	}
	code := []fixtureCode{
		{file: "apps/app/svc/svc.go", name: "Serve"},
		{file: "apps/app/main.go", name: "Main", reads: []string{"apps/app/svc/svc.go"}},
	}

	born := commitOf(t, files, code)

	for _, file := range []string{".gitignore", ".idea/vcs.xml", "Taskfile.yml", "tools/.golangci.yml"} {
		if born[file] != 0 {
			t.Errorf("%s born in commit %d, want the first", file, born[file])
		}
	}

	if born["apps/app/main.go"] == 0 {
		t.Errorf("product code is born in the root commit, beside the scaffold")
	}
}
