package golang

import (
	"context"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// migrationRule orders the statements of a migration against what they build
// on: a statement after the table it adds to, a foreign key after the table
// it points at, a table after the ORM type it was generated from. Placed by
// file, 9 of 12 migrations landed ahead of their own source, one by 735
// commits, and the 12 files arrived in a single commit at the end.
type migrationRule struct{}

func (migrationRule) Name() string { return "migration" }

func (migrationRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	model, err := loadSQLModel(snap, decls)
	if err != nil {
		return nil, err
	}

	var edges []entities.Edge

	for _, name := range model.order {
		edges = append(edges, model.tableEdges(model.tables[name])...)
	}

	return append(edges, model.genesisEdges(snap, decls)...), nil
}

// tableEdges are the table's own: its later statements wait for the CREATE
// TABLE, the CREATE TABLE for its ORM type, and a statement holding a foreign
// key for the CREATE TABLE of the table it references.
func (model sqlModel) tableEdges(table *sqlTable) []entities.Edge {
	create, created := table.create()
	if !created {
		return nil
	}

	var edges []entities.Edge

	if table.entType.Name != "" {
		edges = append(edges, schemaEdge(create, table.entType))
	}

	for _, stmt := range table.stmts {
		if stmt.key != create {
			edges = append(edges, schemaEdge(stmt.key, create))
		}

		for _, ref := range stmt.refs {
			if target, known := model.tables[ref]; known {
				targetCreate, _ := target.create()
				edges = append(edges, schemaEdge(stmt.key, targetCreate))
			}
		}
	}

	return edges
}

// genesisEdges keep the Go files that embed or apply a migration directory
// behind the first table: the directory must not exist before it does.
func (model sqlModel) genesisEdges(
	snap entities.Snapshot,
	decls []entities.Decl,
) []entities.Edge {
	genesis, found := genesisOf(snap, model)
	if !found {
		return nil
	}

	create, _ := genesis.table.create()

	var edges []entities.Edge

	for _, decl := range decls {
		if isGoIn(decl.Key.File, path.Dir(create.File)) {
			edges = append(edges, schemaEdge(decl.Key, create))
		}
	}

	return edges
}

func isGoIn(file, dir string) bool {
	return strings.HasSuffix(file, ".go") && path.Dir(file) == dir
}

func schemaEdge(dependent, provider entities.DeclKey) entities.Edge {
	return entities.Edge{Dependent: dependent, Provider: provider, Reason: entities.ReasonSchema}
}

func (migrationRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

func migrationFiles(files []string) []string {
	var out []string

	for _, f := range files {
		if strings.Contains(f, "migrations/") && strings.HasSuffix(f, ".sql") {
			out = append(out, f)
		}
	}

	return out
}
