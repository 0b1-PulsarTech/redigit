package chunk

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSQLQueriesHandles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{"database queries dir", "database/queries/lead.sql", true},
		{"nested queries dir", "modules/foo/database/queries/x.sql", true},
		{"not a queries dir", "database/migrations/0001.sql", false},
		{"not sql", "database/queries/readme.md", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := (SQLQueries{}).Handles(testCase.path); got != testCase.want {
				t.Fatalf("Handles(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestSQLQueriesChunks(t *testing.T) {
	t.Parallel()

	const path = "database/queries/lead.sql"

	testCases := []struct {
		name      string
		source    string
		wantNames []string
	}{
		{
			name:      "empty file falls back to whole file",
			source:    "",
			wantNames: []string{fileDeclName},
		},
		{
			name:      "no name marker falls back to whole file",
			source:    "SELECT 1;\n",
			wantNames: []string{fileDeclName},
		},
		{
			name: "two queries separated by a blank line",
			source: "-- name: GetLeadByID :one\n" +
				"SELECT id FROM leads WHERE id = ?;\n" +
				"\n" +
				"-- name: ListLeadsByOrg :many\n" +
				"SELECT id FROM leads WHERE org_id = ?;\n",
			wantNames: []string{"GetLeadByID", "ListLeadsByOrg"},
		},
		{
			name: "two queries with no blank line between them",
			source: "-- name: GetLeadByID :one\n" +
				"SELECT id FROM leads WHERE id = ?;\n" +
				"-- name: ListLeadsByOrg :many\n" +
				"SELECT id FROM leads WHERE org_id = ?;\n",
			wantNames: []string{"GetLeadByID", "ListLeadsByOrg"},
		},
		{
			name: "a leading comment travels with its query",
			source: "-- Leads are the funnel's entry point.\n" +
				"-- name: GetLeadByID :one\n" +
				"SELECT id FROM leads WHERE id = ?;\n",
			wantNames: []string{"GetLeadByID"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)

			decls, err := (SQLQueries{}).Chunks(context.Background(), path, source)
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			assertReconstructs(t, testCase.source, decls)
			assertDeclNames(t, decls, testCase.wantNames)

			for i, decl := range decls {
				if decl.Key.File != path {
					t.Fatalf("decl[%d].Key.File = %q, want %q", i, decl.Key.File, path)
				}

				if decl.Unit != entities.UnitID(filepath.Dir(path)) {
					t.Fatalf("decl[%d].Unit = %q, want %q", i, decl.Unit, filepath.Dir(path))
				}
			}
		})
	}
}

// TestSQLQueriesLeadingCommentSpan asserts the trap the task calls out by
// name: the span of a query with a leading, contiguous comment must START at
// the comment, not at the "-- name:" line, or the comment ends up describing
// SQL that render() would leave out.
func TestSQLQueriesLeadingCommentSpan(t *testing.T) {
	t.Parallel()

	source := "-- Leads are the funnel's entry point.\n" +
		"-- name: GetLeadByID :one\n" +
		"SELECT id FROM leads WHERE id = ?;\n"

	decls, err := (SQLQueries{}).Chunks(context.Background(), "x.sql", []byte(source))
	if err != nil {
		t.Fatalf("Chunks() error = %v", err)
	}

	if len(decls) != 1 {
		t.Fatalf("Chunks() = %d decls, want 1", len(decls))
	}

	if decls[0].Span.Start != 0 {
		t.Fatalf("Span.Start = %d, want 0 (the comment line)", decls[0].Span.Start)
	}
}

// A comment block separated from the first query by a blank line reads as a
// file header, not that query's doc comment, so it must not attach the way a
// contiguous one does - even though both land inside the first query's span,
// there being no earlier declaration to claim them.
func TestSQLQueriesCommentSeparatedByBlankLineDoesNotAttach(t *testing.T) {
	t.Parallel()

	source := "-- File-level header.\n" +
		"\n" +
		"-- name: GetLeadByID :one\n" +
		"SELECT id FROM leads WHERE id = ?;\n"

	decls, err := (SQLQueries{}).Chunks(context.Background(), "x.sql", []byte(source))
	if err != nil {
		t.Fatalf("Chunks() error = %v", err)
	}

	assertReconstructs(t, source, decls)

	if len(decls) != 1 || decls[0].Span.Start != 0 {
		t.Fatalf("decls = %+v, want a single decl starting at 0", decls)
	}
}

func TestSQLQueriesChunksSelector(t *testing.T) {
	t.Parallel()

	source := "-- name: GetLeadByID :one\nSELECT 1;\n"

	decls, err := (SQLQueries{}).Chunks(context.Background(), "x.sql", []byte(source))
	if err != nil {
		t.Fatalf("Chunks() error = %v", err)
	}

	want := entities.Location{File: "x.sql", Line: 0, Char: len("-- name: ")}
	if decls[0].Selector != want {
		t.Fatalf("Selector = %+v, want %+v", decls[0].Selector, want)
	}
}
