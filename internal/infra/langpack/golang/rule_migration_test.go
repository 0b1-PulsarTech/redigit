package golang

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestMigrationRuleLandsAfterSchemaBeforeQuery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	schemaFile := filepath.Join("database", "entschema", "user.go")
	migrationFile := filepath.Join("database", "migrations", "0001_users.sql")
	queryFile := filepath.Join("database", "queries", "user.sql")

	writeFile(t, root, schemaFile, "package entschema\n\ntype User struct {\n\tName string\n}\n")
	writeFile(t, root, migrationFile, "CREATE TABLE IF NOT EXISTS `users` (id INTEGER);\n")
	writeFile(t, root, queryFile, "-- name: GetUser :one\nSELECT id FROM users WHERE id = ?;\n")

	snap := entities.Snapshot{Root: root, Files: []string{migrationFile, queryFile}}
	syntax := NewSyntax()
	decls := declareAll(t, syntax, root, []string{schemaFile})

	edges, err := migrationRule{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	migrationKey := entities.DeclKey{File: migrationFile, Kind: entities.KindOther, Name: "users"}
	schemaKey := entities.DeclKey{File: schemaFile, Kind: entities.KindType, Name: "User"}
	queryKey := entities.DeclKey{File: queryFile, Kind: entities.KindOther, Name: "GetUser"}

	testCases := []struct {
		name string
		want entities.Edge
	}{
		{
			"migration after schema",
			entities.Edge{
				Dependent: migrationKey,
				Provider:  schemaKey,
				Reason:    entities.ReasonSchema,
			},
		},
		{
			"query after migration",
			entities.Edge{
				Dependent: queryKey,
				Provider:  migrationKey,
				Reason:    entities.ReasonQuery,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !slices.Contains(edges, testCase.want) {
				t.Fatalf("missing edge %+v; got %+v", testCase.want, edges)
			}
		})
	}
}

func TestMigrationRuleNoSchemaMatchStillLinksQuery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(
		t,
		root,
		filepath.Join("database", "migrations", "0001_orders.sql"),
		"CREATE TABLE orders (id INTEGER);\n",
	)
	writeFile(t, root, filepath.Join("database", "queries", "order.sql"), `-- name: GetOrder :one
SELECT id FROM orders WHERE id = ?;
`)

	snap := entities.Snapshot{
		Root: root,
		Files: []string{
			filepath.Join("database", "migrations", "0001_orders.sql"),
			filepath.Join("database", "queries", "order.sql"),
		},
	}

	edges, err := migrationRule{}.Edges(context.Background(), snap, nil)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	migrationKey := entities.DeclKey{
		File: filepath.Join("database", "migrations", "0001_orders.sql"),
		Kind: entities.KindOther,
		Name: "orders",
	}
	queryKey := entities.DeclKey{
		File: filepath.Join("database", "queries", "order.sql"),
		Kind: entities.KindOther,
		Name: "GetOrder",
	}
	want := entities.Edge{Dependent: queryKey, Provider: migrationKey, Reason: entities.ReasonQuery}

	if !slices.Contains(edges, want) {
		t.Fatalf("missing query edge %+v with no schema match; got %+v", want, edges)
	}
}

func TestCreateTablePattern(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		sql  string
		want string
	}{
		{"plain", "CREATE TABLE orders (id INTEGER);", "orders"},
		{"if not exists", "CREATE TABLE IF NOT EXISTS orders (id INTEGER);", "orders"},
		{"backticked", "CREATE TABLE `orders` (id INTEGER);", "orders"},
		{"lowercase keyword", "create table orders (id integer);", "orders"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			match := createTablePattern.FindStringSubmatch(testCase.sql)
			if match == nil {
				t.Fatalf("no match for %q", testCase.sql)
			}

			if match[1] != testCase.want {
				t.Fatalf("table = %q, want %q", match[1], testCase.want)
			}
		})
	}
}
