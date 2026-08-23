package config

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"

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

	// The kept require survives and the dropped one does not, and the result
	// is what `go mod edit` would write - not the kept lines concatenated,
	// which would leave the require block's header without its closing paren.
	want := "module github.com/org/app\n\ngo 1.26\n\nrequire github.com/a/a v1.0.0\n"
	if string(got) != want {
		t.Fatalf("Render() =\n%q\nwant\n%q", got, want)
	}
}

// The defect a full-set round trip cannot reach: a go.mod's require and
// replace lines sit inside block headers that belong to no single line, so a
// SUBSET concatenated textually puts a replace inside the require block and
// the file stops parsing. On a real repository that broke 91 of 97 trees.
func TestSyntaxRendersAParsableSubsetOfABlockStructuredFile(t *testing.T) {
	t.Parallel()

	source := []byte("module github.com/org/app\n\ngo 1.26\n\n" +
		"require (\n\tgithub.com/a/a v1.0.0\n\tgithub.com/b/b v1.0.0\n)\n\n" +
		"replace (\n\tgithub.com/a/a => ../a\n\tgithub.com/b/b => ../b\n)\n")

	syntax := NewSyntax(chunkers())

	decls, err := syntax.Declarations(t.Context(), "go.mod", source)
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	keep := keysOf(decls,
		"github.com/org/app", "require github.com/a/a", "replace github.com/a/a")
	if len(keep) != 3 {
		t.Fatalf("the chunking produced %d of the 3 expected pieces", len(keep))
	}

	got, err := syntax.Render(t.Context(), source, keep)
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}

	if _, err = modfile.Parse("go.mod", got, nil); err != nil {
		t.Fatalf("the rendered subset does not parse: %v\n%s", err, got)
	}

	for _, dropped := range []string{"github.com/b/b"} {
		if strings.Contains(string(got), dropped) {
			t.Fatalf("%s was not kept and is still in the file:\n%s", dropped, got)
		}
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
			source: "module m\n\ngo 1.26\n\n" +
				"require (\n\tgithub.com/a/a v1.0.0\n\tgithub.com/b/b v1.0.0\n)\n\n" +
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
			name:   "a file no chunker splits, with no trailing newline",
			path:   "go.sum",
			source: "github.com/a/a v1.0.0 h1:abc=",
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
