package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestMigrationRuleEdges(t *testing.T) {
	t.Parallel()

	snap, decls := schemaTree(t)

	edges, err := Migration{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	const migration = "database/migrations/0001_all.sql"

	accounts := testkit.DeclKey(migration, entities.KindOther, "table:accounts")
	templates := testkit.DeclKey(migration, entities.KindOther, "table:templates")
	foreignKey := testkit.DeclKey(migration, entities.KindOther, "alter:templates")
	index := testkit.DeclKey(migration, entities.KindOther, "index:account_id")

	testCases := []struct {
		name string
		want entities.Edge
	}{
		{
			"a table after the ORM type it was generated from",
			schemaEdge(
				accounts,
				testkit.DeclKey("database/entschema/account.go", entities.KindType, "Account"),
			),
		},
		{"an index after its table", schemaEdge(index, accounts)},
		{"an alter after its table", schemaEdge(foreignKey, templates)},
		{"a foreign key after the table it points at", schemaEdge(foreignKey, accounts)},
		{
			"a table after the extension defining a type it uses",
			schemaEdge(
				templates,
				testkit.DeclKey(migration, entities.KindOther, "extension:unaccent"),
			),
		},
		{
			"the migrations package after the first table",
			schemaEdge(
				testkit.DeclKey("database/migrations/embed.go", entities.KindValue, "FS"),
				accounts,
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

func TestMigrationRuleWithoutMigrationsHasNoEdges(t *testing.T) {
	t.Parallel()

	edges, err := Migration{}.Edges(
		context.Background(), entities.Snapshot{Root: t.TempDir()}, nil,
	)
	if err != nil || len(edges) != 0 {
		t.Fatalf("Edges() = %v, %v; want none", edges, err)
	}
}
