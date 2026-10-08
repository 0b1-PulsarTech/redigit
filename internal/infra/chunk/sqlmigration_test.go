package chunk

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSQLMigrationsHandles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{"database migrations dir", "database/migrations/0001_users.sql", true},
		{"nested migrations dir", "modules/foo/migrations/x.sql", true},
		{"a queries dir", "database/queries/lead.sql", false},
		{"not sql", "database/migrations/atlas.sum", false},
		{"a seed script", "database/local.newdb-init/init.sql", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := (SQLMigrations{}).Handles(testCase.path); got != testCase.want {
				t.Fatalf("Handles(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestSQLMigrationsChunks(t *testing.T) {
	t.Parallel()

	const path = "database/migrations/0001_users.sql"

	testCases := []struct {
		name      string
		source    string
		wantNames []string
	}{
		{"empty file is one whole-file piece", "", []string{fileDeclName}},
		{"comment only is one whole-file piece", "-- nothing\n", []string{fileDeclName}},
		{
			name: "a table, its index and a later table",
			source: "-- Create \"users\" table\n" +
				"CREATE TABLE \"users\" (\"id\" uuid NOT NULL);\n" +
				"-- Create index \"user_id\" to table: \"users\"\n" +
				"CREATE INDEX \"user_id\" ON \"users\" (\"id\");\n" +
				"-- Create \"roles\" table\n" +
				"CREATE TABLE \"roles\" (\"id\" uuid NOT NULL);\n",
			wantNames: []string{"table:users", "index:user_id", "table:roles"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := (SQLMigrations{}).Chunks(
				context.Background(), path, []byte(testCase.source),
			)
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			assertReconstructs(t, testCase.source, decls)
			assertDeclNames(t, decls, testCase.wantNames)
		})
	}
}

// Two tables of one migration are born in different commits: rendering the
// file with one of them kept is a file the migration tool still reads.
func TestSQLMigrationsRenderOneTable(t *testing.T) {
	t.Parallel()

	const (
		path   = "database/migrations/0001_users.sql"
		source = "-- Create \"users\" table\nCREATE TABLE \"users\" (\"id\" uuid);\n" +
			"-- Create \"roles\" table\nCREATE TABLE \"roles\" (\"id\" uuid);\n"
	)

	decls, err := (SQLMigrations{}).Chunks(context.Background(), path, []byte(source))
	if err != nil {
		t.Fatalf("Chunks() error = %v", err)
	}

	var kept entities.Span

	for _, decl := range decls {
		if decl.Key.Name == "table:roles" {
			kept = decl.Span
		}
	}

	want := "-- Create \"roles\" table\nCREATE TABLE \"roles\" (\"id\" uuid);\n"
	if got := source[kept.Start:kept.End]; got != want {
		t.Fatalf("roles span = %q, want %q", got, want)
	}
}
