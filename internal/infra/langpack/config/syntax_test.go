package config

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/chunk"
)

func chunkers() []Chunker {
	out := make([]Chunker, 0, len(chunk.Registry))
	for _, chunker := range chunk.Registry {
		out = append(out, chunker)
	}

	return out
}

func keysOf(decls []entities.Decl, names ...string) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(names))

	for _, want := range names {
		for _, decl := range decls {
			if decl.Key.Name == want {
				out = append(out, decl.Key)

				break
			}
		}
	}

	return out
}

// A commit that carries three of a file's pieces must produce exactly those
// three bytes, in file order. This is the whole of rendering a chunked file:
// the pieces tile it end to end, so a subset concatenated in span order is the
// answer.
func TestSyntaxRendersOnlyTheKeptPieces(t *testing.T) {
	t.Parallel()

	source := []byte("module github.com/org/app\n\ngo 1.26\n\n" +
		"require (\n\tgithub.com/a/a v1.0.0\n\tgithub.com/b/b v1.0.0\n)\n")

	syntax := NewSyntax(chunkers())

	decls, err := syntax.Declarations(t.Context(), "go.mod", source)
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	keep := keysOf(decls, "github.com/org/app", "require github.com/a/a")
	if len(keep) != 2 {
		t.Fatalf("the chunking produced %d of the 2 expected pieces", len(keep))
	}

	got, err := syntax.Render(t.Context(), source, keep)
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}

	want := "module github.com/org/app\n\ngo 1.26\n\nrequire (\n\tgithub.com/a/a v1.0.0\n"
	if string(got) != want {
		t.Fatalf("Render() =\n%q\nwant\n%q", got, want)
	}
}

// Keeping every piece must reproduce the file byte for byte. If it does not,
// the final tree cannot match, and the failure would surface only at the very
// end as a tree that diverges with no explanation left.
func TestSyntaxRenderingEveryPieceReproducesTheFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		path   string
		source string
	}{
		{
			name: "a go.mod with a require block and a replace",
			path: "go.mod",
			source: "module m\n\ngo 1.26\n\nrequire (\n\tgithub.com/a/a v1.0.0\n)\n\n" +
				"replace github.com/a/a => ../a\n",
		},
		{
			name:   "a go.work",
			path:   "go.work",
			source: "go 1.26\n\nuse (\n\t./a\n\t./b\n)\n",
		},
		{
			name:   "a query file",
			path:   "database/queries/lead.sql",
			source: "-- name: GetLead :one\nSELECT 1;\n\n-- name: ListLeads :many\nSELECT 2;\n",
		},
		{
			name:   "a file no chunker splits",
			path:   "go.sum",
			source: "github.com/a/a v1.0.0 h1:abc=\n",
		},
		{
			name:   "a file with no trailing newline",
			path:   "go.work",
			source: "go 1.26\n\nuse ./a",
		},
	}

	syntax := NewSyntax(chunkers())

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)

			decls, err := syntax.Declarations(t.Context(), testCase.path, source)
			if err != nil {
				t.Fatalf("Declarations() = %v", err)
			}

			all := make([]entities.DeclKey, 0, len(decls))
			for _, decl := range decls {
				all = append(all, decl.Key)
			}

			got, err := syntax.Render(t.Context(), source, all)
			if err != nil {
				t.Fatalf("Render() = %v", err)
			}

			if string(got) != testCase.source {
				t.Fatalf("rendering every piece gave\n%q\nwant\n%q", got, testCase.source)
			}
		})
	}
}

// A path no chunker claims yields no declaration, which leaves the file out of
// the graph rather than inventing a shape for it. WholeFile claims everything,
// so this needs a syntax built without it.
func TestSyntaxWithNoMatchingChunkerYieldsNothing(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax([]Chunker{chunk.GoMod{}})

	decls, err := syntax.Declarations(context.Background(), "README.md", []byte("# title\n"))
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	if len(decls) != 0 {
		t.Fatalf("got %d declaration(s) for an unclaimed path", len(decls))
	}
}

func TestSyntaxRenderWithNothingKeptIsEmpty(t *testing.T) {
	t.Parallel()

	got, err := NewSyntax(chunkers()).Render(t.Context(), []byte("module m\n"), nil)
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("Render() = %q, want empty", got)
	}
}
