package golang

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSQLAtomRuleAtoms(t *testing.T) {
	t.Parallel()

	snap, decls := schemaTree(t)

	atoms, err := sqlAtomRule{}.Atoms(context.Background(), snap, decls)
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

	atoms, err := sqlAtomRule{}.Atoms(
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
			key(migration, entities.KindOther, "table:templates"),
			key("database/entschema/template.go", entities.KindType, "Template"),
			true,
		},
		{
			"a table with its index",
			key(migration, entities.KindOther, "table:templates"),
			key(migration, entities.KindOther, "alter:templates"),
			true,
		},
		{
			"a table with its generated struct",
			key(migration, entities.KindOther, "table:templates"),
			key("dbgen/models.go", entities.KindType, "Template"),
			true,
		},
		{
			"a query with its function and row type",
			key("database/queries/template.sql", entities.KindOther, "ListTemplates"),
			key("dbgen/template.sql.go", entities.KindType, "ListTemplatesRow"),
			true,
		},
		{
			"a later table apart from the first",
			key(migration, entities.KindOther, "table:templates"),
			key(migration, entities.KindOther, "table:accounts"),
			false,
		},
		{
			"the first table with the package root",
			key(migration, entities.KindOther, "table:accounts"),
			key("dbgen/db.go", entities.KindType, "Queries"),
			true,
		},
		{
			"the first table with a query that reads only it",
			key(migration, entities.KindOther, "table:accounts"),
			key("database/queries/account.sql", entities.KindOther, "GetAccount"),
			true,
		},
		{
			"the first table with the ORM helpers it stands on",
			key(migration, entities.KindOther, "table:accounts"),
			key("database/entschema/customixins/uuid.go", entities.KindFunc, "UUID"),
			true,
		},
		{
			"the first table with the sqlc configuration",
			key(migration, entities.KindOther, "table:accounts"),
			key("database/sqlc.yaml", entities.KindOther, fileDeclName),
			true,
		},
		{
			"the first table with the migration checksum",
			key(migration, entities.KindOther, "table:accounts"),
			key("database/migrations/atlas.sum", entities.KindOther, fileDeclName),
			true,
		},
		{
			"a helper that reaches for a table stays out of the first one",
			key(migration, entities.KindOther, "table:accounts"),
			key("database/entschema/owner.go", entities.KindType, "OwnerMixin"),
			false,
		},
	}
}
