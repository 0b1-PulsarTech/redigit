package wiring

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestEmbedRuleMakesTheEmbeddingDeclarationWaitForItsData(t *testing.T) {
	t.Parallel()

	const embedGo = "database/migrations/embed.go"

	embedSource := "package migrations\n\nimport \"embed\"\n\n" +
		"//go:embed *.sql\nvar FS embed.FS\n\nfunc Other() {}\n"

	testCases := []struct {
		name     string
		source   string
		files    map[string]string
		wantFile string
	}{
		{
			name:     "a glob waits for the first file it matches",
			source:   embedSource,
			files:    map[string]string{"0002_b.sql": "x", "0001_a.sql": "x", "README.md": "x"},
			wantFile: "database/migrations/0001_a.sql",
		},
		{
			name: "a directory pattern waits for a file beneath it",
			source: "package migrations\n\nimport \"embed\"\n\n" +
				"//go:embed seed\nvar FS embed.FS\n",
			files:    map[string]string{"seed/a.sql": "x", "seed/_skip.sql": "x"},
			wantFile: "database/migrations/seed/a.sql",
		},
		{
			name: "a quoted pattern with a space",
			source: "package migrations\n\nimport \"embed\"\n\n" +
				"//go:embed \"my data.txt\"\nvar FS embed.FS\n",
			files:    map[string]string{"my data.txt": "x"},
			wantFile: "database/migrations/my data.txt",
		},
		{
			name: "all: reaches a hidden file",
			source: "package migrations\n\nimport \"embed\"\n\n" +
				"//go:embed all:seed\nvar FS embed.FS\n",
			files:    map[string]string{"seed/_a.sql": "x"},
			wantFile: "database/migrations/seed/_a.sql",
		},
		{
			name:   "a pattern matching nothing yields no edge",
			source: embedSource,
			files:  map[string]string{"README.md": "x"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			testkit.WriteFile(t, root, embedGo, testCase.source)

			paths := []string{embedGo}

			var data []entities.Decl

			for name := range testCase.files {
				rel := "database/migrations/" + name
				testkit.WriteFile(t, root, rel, testCase.files[name])

				paths = append(paths, rel)
				data = append(data, entities.Decl{
					Key: entities.DeclKey{File: rel, Kind: entities.KindOther, Name: "<file>"},
				})
			}

			decls := append(testkit.DeclareAll(t, root, []string{embedGo}), data...)
			snap := entities.Snapshot{Root: root, Files: paths}

			edges, err := Embed{}.Edges(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Edges() error = %v", err)
			}

			assertEmbedEdges(t, edges, testCase.wantFile)
		})
	}
}

func assertEmbedEdges(t *testing.T, edges []entities.Edge, wantFile string) {
	t.Helper()

	if wantFile == "" {
		if len(edges) != 0 {
			t.Fatalf("got %d edge(s), want none: %v", len(edges), edges)
		}

		return
	}

	if len(edges) != 1 {
		t.Fatalf("got %d edge(s), want exactly the embedding declaration's: %v", len(edges), edges)
	}

	edge := edges[0]
	if edge.Dependent.Name != "FS" || edge.Provider.File != wantFile {
		t.Fatalf("edge = %s -> %s, want FS -> %s", edge.Dependent, edge.Provider, wantFile)
	}
}
