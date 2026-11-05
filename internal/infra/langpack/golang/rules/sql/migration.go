package sql

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
	"github.com/wrapped-owls/redigit/internal/infra/sqlstmt"
)

type Migration struct{}

func (Migration) Name() string { return "migration" }

func (Migration) Edges(
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

		for _, word := range stmt.words {
			if extension, isExtension := model.extensions[word]; isExtension {
				edges = append(edges, schemaEdge(stmt.key, extension))
			}
		}
	}

	return edges
}

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

func (Migration) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

func migrationFiles(files []string) []string {
	var out []string

	for _, file := range files {
		if strings.Contains(file, "migrations/") && strings.HasSuffix(file, ".sql") {
			out = append(out, file)
		}
	}

	return out
}

func (model *sqlModel) readMigrations(snap entities.Snapshot) error {
	for _, file := range migrationFiles(snap.Files) {
		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return fmt.Errorf("sql model source: %w", err)
		}

		for _, stmt := range sqlstmt.Split(source) {
			key := entities.DeclKey{File: file, Kind: entities.KindOther, Name: stmt.Name}

			if name, isExtension := strings.CutPrefix(stmt.Name, "extension:"); isExtension {
				model.extensions[name] = key
			}

			if stmt.Kind == sqlstmt.CreateTable {
				model.tables[stmt.Table] = &sqlTable{name: stmt.Table}
				model.order = append(model.order, stmt.Table)
			}

			table, known := model.tables[stmt.Table]
			if !known {
				continue
			}

			table.stmts = append(
				table.stmts, sqlStmt{
					key: key, kind: stmt.Kind, refs: stmt.Refs,
					words: sqlstmt.Idents(source[stmt.Start:stmt.End]),
				},
			)

			for _, ref := range stmt.Refs {
				if ref != stmt.Table && !slices.Contains(table.refs, ref) {
					table.refs = append(table.refs, ref)
				}
			}
		}
	}

	return nil
}
