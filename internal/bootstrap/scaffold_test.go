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

	snap, decls := writeFixture(t, files)

	for _, each := range code {
		decls = append(decls, goDecl(each.file, each.name))
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

	atoms, err := pack.Atoms(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Atoms() = %v", err)
	}

	for index := range decls {
		if name, grouped := atoms[decls[index].Key]; grouped && decls[index].Atom == "" {
			decls[index].Atom = name
		}
	}

	graph := entities.NewGraph(decls, append(readEdges(code), found...), anchors)

	order, err := ordering.New(ordering.Config{}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	plan, err := planning.New(planning.Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	return firstCommits(plan)
}

// writeFixture puts the files on disk, where the rules read them, and chunks
// each as the indexer would.
func writeFixture(t *testing.T, files map[string]string) (entities.Snapshot, []entities.Decl) {
	t.Helper()

	snap := entities.Snapshot{Root: t.TempDir()}

	decls := make([]entities.Decl, 0, len(files))

	for name, body := range files {
		full := filepath.Join(snap.Root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}

		snap.Files = append(snap.Files, name)
		decls = append(decls, wholeFiles(t, name, body)...)
	}

	return snap, decls
}

func readEdges(code []fixtureCode) []entities.Edge {
	byName := map[string]fixtureCode{}

	for _, each := range code {
		byName[each.name] = each
	}

	var edges []entities.Edge

	for _, each := range code {
		for _, read := range each.reads {
			edges = append(edges, entities.Edge{
				Dependent: goDecl(each.file, each.name).Key,
				Provider:  goDecl(byName[read].file, read).Key,
				Reason:    entities.ReasonReference,
			})
		}
	}

	return edges
}

func firstCommits(plan entities.Plan) map[string]entities.Position {
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
		{file: "apps/app/main.go", name: "Main", reads: []string{"Serve"}},
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

func TestModuleFilesAreBornWithTheirGoMod(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"apps/app/go.mod":       "module example.com/app\n\ngo 1.26\n",
		"apps/app/README.md":    "# app\n",
		"apps/app/Taskfile.yml": "version: '3'\n",
		"apps/lib/go.mod":       "module example.com/lib\n\ngo 1.26\n",
		"apps/lib/README.md":    "# lib\n",
		"go.work":               "go 1.26\n\nuse (\n\t./apps/app\n\t./apps/lib\n)\n",
	}
	code := []fixtureCode{
		{file: "apps/lib/lib.go", name: "Helper"},
		{file: "apps/app/svc/svc.go", name: "Serve", reads: []string{"Helper"}},
		{file: "apps/app/main.go", name: "Main", reads: []string{"Serve"}},
	}

	born := commitOf(t, files, code)

	for _, module := range []string{"apps/app", "apps/lib"} {
		for _, file := range []string{"README.md", "Taskfile.yml"} {
			path := module + "/" + file
			if _, exists := files[path]; !exists {
				continue
			}

			if born[path] != born[module+"/go.mod"] {
				t.Errorf(
					"%s born in commit %d, its go.mod in %d",
					path,
					born[path],
					born[module+"/go.mod"],
				)
			}
		}
	}

	if born["apps/app/go.mod"] > born["apps/app/main.go"] {
		t.Errorf("go.mod is born after the module's code: %v", born)
	}
}

func TestDocumentIsBornAfterTheCodeItNames(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"apps/app/go.mod":           "module example.com/app\n\ngo 1.26\n",
		"docs/worker.md":            "The queue consumer lives in `apps/app/worker`.\n",
		"apps/app/worker/README.md": "# worker\n",
	}
	code := []fixtureCode{
		{file: "apps/app/svc/svc.go", name: "Serve"},
		{file: "apps/app/main.go", name: "Main", reads: []string{"Serve"}},
		{file: "apps/app/worker/queue.go", name: "Queue"},
		{file: "apps/app/worker/worker.go", name: "Work", reads: []string{"Queue"}},
	}

	born := commitOf(t, files, code)

	last := max(born["apps/app/worker/queue.go"], born["apps/app/worker/worker.go"])

	// A spec is true of the code it names, so it is a commit of documents right
	// after that code and never a preamble to it.
	if born["docs/worker.md"] <= last {
		t.Errorf(
			"docs/worker.md born in commit %d, the code it names in %d: a document opened the history",
			born["docs/worker.md"],
			last,
		)
	}

	if born["apps/app/worker/README.md"] != min(
		born["apps/app/worker/queue.go"], born["apps/app/worker/worker.go"],
	) {
		t.Errorf(
			"the package README born in commit %d, the package's first code in %d",
			born["apps/app/worker/README.md"],
			born["apps/app/worker/queue.go"],
		)
	}
}
