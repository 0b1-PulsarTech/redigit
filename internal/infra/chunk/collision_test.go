package chunk

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// A go.work repository requires a sibling module and replaces it with the
// local path in the same file. Both lines name the same module path, so a
// Decl named after the module path alone gives the two lines one DeclKey -
// and a map keyed by DeclKey keeps only one of them, dropping the other
// line's bytes from every commit that should have carried it.
func TestGoModRequireAndReplaceOfSameModuleAreDistinctDecls(t *testing.T) {
	t.Parallel()

	source := []byte(`module github.com/acme/app

go 1.24

require (
	github.com/acme/lib v0.0.0
)

replace (
	github.com/acme/lib => ../lib
)
`)

	decls, err := GoMod{}.Chunks(t.Context(), "apps/app/go.mod", source)
	if err != nil {
		t.Fatalf("Chunks() = %v", err)
	}

	seen := map[entities.DeclKey]entities.Span{}

	for _, decl := range decls {
		if prev, dup := seen[decl.Key]; dup {
			t.Fatalf(
				"require and replace of %q share one DeclKey: spans %v and %v collapse onto "+
					"each other, so one line's bytes never reach a commit",
				decl.Key.Name, prev, decl.Span,
			)
		}

		seen[decl.Key] = decl.Span
	}
}

// Whatever a chunker names its Decls, two things must always hold: no two
// share a DeclKey, and concatenating their spans in file order reproduces the
// source byte for byte. A chunker that violates either one silently drops
// bytes from the emitted tree.
func TestChunkersLoseNoByteAndCollideOnNoKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		chunker indexing.Chunker
		path    string
		source  string
	}{
		{
			name:    "go.mod with require and replace of the same module",
			chunker: GoMod{},
			path:    "apps/app/go.mod",
			source: "module github.com/acme/app\n\ngo 1.24\n\nrequire (\n\tgithub.com/acme/lib v0.0.0\n)\n" +
				"\nreplace (\n\tgithub.com/acme/lib => ../lib\n)\n",
		},
		{
			name:    "go.mod single-line require and replace of the same module",
			chunker: GoMod{},
			path:    "go.mod",
			source: "module m\n\ngo 1.24\n\nrequire github.com/acme/lib v0.0.0\n" +
				"replace github.com/acme/lib => ../lib\n",
		},
		{
			name:    "go.work using several modules",
			chunker: GoWork{},
			path:    "go.work",
			source:  "go 1.24\n\nuse (\n\t./a\n\t./b\n)\n\nuse ./c\n",
		},
		{
			name:    "query file with a doc comment and a section header",
			chunker: SQLQueries{},
			path:    "database/queries/lead.sql",
			source: "-- section\n\n-- returns one lead\n-- name: GetLead :one\nSELECT 1;\n\n" +
				"-- name: ListLeads :many\nSELECT 2;\n",
		},
		{
			name:    "file no chunker splits",
			chunker: WholeFile{},
			path:    "README.md",
			source:  "# title\n\nprose\n",
		},
		{
			name:    "file with no trailing newline",
			chunker: GoWork{},
			path:    "go.work",
			source:  "go 1.24\n\nuse ./a",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)

			decls, err := testCase.chunker.Chunks(context.Background(), testCase.path, source)
			if err != nil {
				t.Fatalf("Chunks() = %v", err)
			}

			assertNoDuplicateKeys(t, decls)
			assertSpansCoverSource(t, decls, source)
		})
	}
}

func assertNoDuplicateKeys(t *testing.T, decls []entities.Decl) {
	t.Helper()

	seen := map[entities.DeclKey]entities.Span{}

	for _, decl := range decls {
		if prev, dup := seen[decl.Key]; dup {
			t.Fatalf(
				"two Decls share the key %+v: spans %v and %v; one of them is unreachable",
				decl.Key, prev, decl.Span,
			)
		}

		seen[decl.Key] = decl.Span
	}
}

func assertSpansCoverSource(t *testing.T, decls []entities.Decl, source []byte) {
	t.Helper()

	cursor := 0

	for _, decl := range decls {
		if decl.Span.Start != cursor {
			t.Fatalf(
				"span %v does not resume at %d: bytes %q are covered by nothing or by two Decls",
				decl.Span, cursor, source[min(cursor, decl.Span.Start):max(cursor, decl.Span.Start)],
			)
		}

		cursor = decl.Span.End
	}

	if cursor != len(source) {
		t.Fatalf("spans stop at %d of %d bytes: the tail %q reaches no commit",
			cursor, len(source), source[cursor:])
	}
}
