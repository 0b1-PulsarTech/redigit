package golang

import (
	"context"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSqlcRuleEdges(t *testing.T) {
	t.Parallel()

	snap, decls := schemaTree(t)

	edges, err := sqlcRule{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	const migration = "database/migrations/0001_all.sql"

	block := key("database/queries/template.sql", entities.KindOther, "ListTemplates")
	queryEdge := func(dependent, provider entities.DeclKey) entities.Edge {
		return entities.Edge{Dependent: dependent, Provider: provider, Reason: entities.ReasonQuery}
	}

	testCases := []struct {
		name string
		want entities.Edge
	}{
		{
			"a query after each table it reads",
			queryEdge(block, key(migration, entities.KindOther, "table:templates")),
		},
		{
			"a query after the second table it reads",
			queryEdge(block, key(migration, entities.KindOther, "table:accounts")),
		},
		{
			"a query after the extension it calls",
			queryEdge(block, key(migration, entities.KindOther, "extension:unaccent")),
		},
		{
			"a generated function after its query",
			queryEdge(
				key("dbgen/template.sql.go", entities.KindMethod, "Queries.ListTemplates"),
				block,
			),
		},
		{
			"a model after the generated package root",
			queryEdge(
				key("dbgen/models.go", entities.KindType, "Template"),
				key("dbgen/db.go", entities.KindType, "Queries"),
			),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !slices.Contains(edges, testCase.want) {
				t.Fatalf("missing edge %+v in %+v", testCase.want, edges)
			}
		})
	}
}

func TestSqlcRuleNoQueryFilesNoEdges(t *testing.T) {
	t.Parallel()

	edges, err := sqlcRule{}.Edges(context.Background(), entities.Snapshot{Root: t.TempDir()}, nil)
	if err != nil || len(edges) != 0 {
		t.Fatalf("Edges() = %v, %v; want none", edges, err)
	}
}

func TestExtractQueryNames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		want   []string
	}{
		{
			"two queries",
			"-- name: GetUser :one\nSELECT 1;\n\n-- name: ListUsers :many\nSELECT 1;\n",
			[]string{"GetUser", "ListUsers"},
		},
		{"no queries", "SELECT 1;\n", nil},
		{"indented marker is not matched (must be at line start)", "  -- name: Nope :one\n", nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := extractQueryNames([]byte(testCase.source))
			if len(got) != len(testCase.want) {
				t.Fatalf("extractQueryNames() = %v, want %v", got, testCase.want)
			}

			for i, name := range got {
				if name != testCase.want[i] {
					t.Fatalf("extractQueryNames()[%d] = %q, want %q", i, name, testCase.want[i])
				}
			}
		})
	}
}
