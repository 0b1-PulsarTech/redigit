package sql

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestSQLAtomRuleAtoms(t *testing.T) {
	t.Parallel()

	snap, decls := schemaTree(t)

	atoms, err := Atoms{}.Atoms(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Atoms() error = %v", err)
	}

	for _, testCase := range atomCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			left, right := atoms[testCase.key], atoms[testCase.other]
			if testCase.same && (left == "" || left != right) {
				t.Fatalf("atoms = %q and %q, want one shared atom", left, right)
			}

			if !testCase.same && left == right && left != "" {
				t.Fatalf("both in atom %q, want them apart", left)
			}
		})
	}
}

func TestSQLAtomRuleWithoutSchemaGroupsNothing(t *testing.T) {
	t.Parallel()

	atoms, err := Atoms{}.Atoms(
		context.Background(),
		entities.Snapshot{Root: t.TempDir()},
		nil,
	)
	if err != nil || len(atoms) != 0 {
		t.Fatalf("Atoms() = %v, %v; want none", atoms, err)
	}
}

type atomCase struct {
	name  string
	key   entities.DeclKey
	other entities.DeclKey
	same  bool
}

const migration = "database/migrations/0001_all.sql"

func atomCases() []atomCase {
	return []atomCase{
		{
			"a table with its ORM type",
			testkit.DeclKey(migration, entities.KindOther, "table:templates"),
			testkit.DeclKey("database/entschema/template.go", entities.KindType, "Template"),
			true,
		},
		{
			"a table with its index",
			testkit.DeclKey(migration, entities.KindOther, "table:templates"),
			testkit.DeclKey(migration, entities.KindOther, "alter:templates"),
			true,
		},
		{
			"a table with its generated struct",
			testkit.DeclKey(migration, entities.KindOther, "table:templates"),
			testkit.DeclKey("dbgen/models.go", entities.KindType, "Template"),
			true,
		},
		{
			"a query with its function and row type",
			testkit.DeclKey("database/queries/template.sql", entities.KindOther, "ListTemplates"),
			testkit.DeclKey("dbgen/template.sql.go", entities.KindType, "ListTemplatesRow"),
			true,
		},
		{
			"a later table apart from the first",
			testkit.DeclKey(migration, entities.KindOther, "table:templates"),
			testkit.DeclKey(migration, entities.KindOther, "table:accounts"),
			false,
		},
		{
			"the first table with the package root",
			testkit.DeclKey(migration, entities.KindOther, "table:accounts"),
			testkit.DeclKey("dbgen/db.go", entities.KindType, "Queries"),
			true,
		},
		{
			"the first table with a query that reads only it",
			testkit.DeclKey(migration, entities.KindOther, "table:accounts"),
			testkit.DeclKey("database/queries/account.sql", entities.KindOther, "GetAccount"),
			true,
		},
		{
			"the first table with the ORM helpers it stands on",
			testkit.DeclKey(migration, entities.KindOther, "table:accounts"),
			testkit.DeclKey("database/entschema/customixins/uuid.go", entities.KindFunc, "UUID"),
			true,
		},
		{
			"the first table with the sqlc configuration",
			testkit.DeclKey(migration, entities.KindOther, "table:accounts"),
			testkit.DeclKey("database/sqlc.yaml", entities.KindOther, syntax.FileDeclName),
			true,
		},
		{
			"the first table with the module of the sqlc configuration",
			testkit.DeclKey(migration, entities.KindOther, "table:accounts"),
			testkit.DeclKey("database/go.mod", entities.KindOther, "example.com/database"),
			true,
		},
		{
			"the first table with the migration checksum",
			testkit.DeclKey(migration, entities.KindOther, "table:accounts"),
			testkit.DeclKey(
				"database/migrations/atlas.sum",
				entities.KindOther,
				syntax.FileDeclName,
			),
			true,
		},
		{
			"a helper that reaches for a table stays out of the first one",
			testkit.DeclKey(migration, entities.KindOther, "table:accounts"),
			testkit.DeclKey("database/entschema/owner.go", entities.KindType, "OwnerMixin"),
			false,
		},
	}
}
