package sqlstmt

import (
	"slices"
	"testing"
)

func TestSplit(t *testing.T) {
	t.Parallel()

	const source = `-- Create "a" table
CREATE TABLE "a" ("id" uuid NOT NULL, PRIMARY KEY ("id"));
-- Create index "a_idx" to table: "a"
CREATE UNIQUE INDEX "a_idx" ON "a" ("id");
-- note; with a semicolon
-- Create "b" table
CREATE TABLE "b" ("a_id" uuid NOT NULL, "x" text DEFAULT 'a;b',
  CONSTRAINT "fk" FOREIGN KEY ("a_id") REFERENCES "public"."a" ("id"));
CREATE EXTENSION IF NOT EXISTS "unaccent";
ALTER TABLE "b" ADD COLUMN "c_id" uuid NULL, ADD CONSTRAINT "fk2" FOREIGN KEY ("c_id") REFERENCES "c" ("id");
ALTER TABLE "b" ADD COLUMN "d" int NULL;
SELECT 1;
`

	got := Split([]byte(source))

	want := []struct {
		name  string
		kind  Kind
		table string
		refs  []string
	}{
		{"table:a", CreateTable, "a", nil},
		{"index:a_idx", CreateIndex, "a", nil},
		{"table:b", CreateTable, "b", []string{"a"}},
		{"extension:unaccent", Extension, "", nil},
		{"alter:b", AlterTable, "b", []string{"c"}},
		{"alter:b#2", AlterTable, "b", nil},
		{"stmt:7", Other, "", nil},
	}

	if len(got) != len(want) {
		t.Fatalf("Split() found %d statements, want %d: %+v", len(got), len(want), got)
	}

	for index, testCase := range want {
		stmt := got[index]
		if stmt.Name != testCase.name || stmt.Kind != testCase.kind ||
			stmt.Table != testCase.table || !slices.Equal(stmt.Refs, testCase.refs) {
			t.Errorf("statement %d = %+v, want %+v", index, stmt, testCase)
		}
	}

	next := 0
	for index, stmt := range got {
		if stmt.Start != next {
			t.Errorf("statement %d starts at %d, want %d: the statements must tile the file",
				index, stmt.Start, next)
		}

		next = stmt.End
	}

	if next != len(source) {
		t.Errorf("statements end at %d, want %d", next, len(source))
	}
}

func TestSplitKeepsTheCommentWithItsStatement(t *testing.T) {
	t.Parallel()

	const source = "-- first\nCREATE TABLE \"a\" ();\n-- second\n-- more\nCREATE TABLE \"b\" ();\n"

	got := Split([]byte(source))
	if len(got) != 2 {
		t.Fatalf("Split() found %d statements, want 2", len(got))
	}

	if text := source[got[1].Start:got[1].End]; text != "-- second\n-- more\nCREATE TABLE \"b\" ();\n" {
		t.Errorf("second statement = %q, want its two comment lines and the statement", text)
	}
}

func TestSplitFindsNothingInAFileWithoutStatements(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
	}{
		{"empty", ""},
		{"only a comment", "-- nothing here\n"},
		{"no terminator", "CREATE TABLE \"a\" ()"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Split([]byte(testCase.source)); len(got) != 0 {
				t.Fatalf("Split() = %+v, want none", got)
			}
		})
	}
}

func TestIdents(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string
		want []string
	}{
		{
			"skips comments and strings",
			"-- name: GetUser :one\nSELECT u.id FROM users u WHERE u.kind = 'leads'",
			[]string{"select", "u", "id", "from", "users", "u", "where", "u", "kind"},
		},
		{"unquotes", `SELECT "Id" FROM "users"`, []string{"select", "id", "from", "users"}},
		{"doubled quote", `"a""b"`, []string{`a"b`}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Idents([]byte(testCase.text)); !slices.Equal(got, testCase.want) {
				t.Fatalf("Idents() = %v, want %v", got, testCase.want)
			}
		})
	}
}
