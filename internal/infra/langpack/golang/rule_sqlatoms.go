package golang

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// sqlAtomRule groups what one schema change touches into one commit: a table
// is its ORM file, its statements and its generated struct; a query is its
// block and the functions sqlc wrote for it. Neither side compiles or applies
// alone - a struct with no table, or a table the ORM has no type for, is a
// tree schemadiff or sqlc rejects.
type sqlAtomRule struct{}

func (sqlAtomRule) Name() string { return "sql-atoms" }

func (sqlAtomRule) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

func (sqlAtomRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

func (sqlAtomRule) Atoms(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	model, err := loadSQLModel(snap, decls)
	if err != nil {
		return nil, err
	}

	atoms := map[entities.DeclKey]string{}
	genesis, hasGenesis := genesisOf(snap, model)

	for _, query := range model.queries {
		label := "query:" + query.name

		if hasGenesis && query == genesis.query {
			continue
		}

		for _, key := range append([]entities.DeclKey{query.block}, query.generated...) {
			atoms[key] = label
		}
	}

	for _, name := range model.order {
		table := model.tables[name]
		label := "table:" + name

		for _, stmt := range table.stmts {
			atoms[stmt.key] = label
		}

		for _, key := range append([]entities.DeclKey{table.model}, table.ent...) {
			if key != (entities.DeclKey{}) {
				atoms[key] = label
			}
		}
	}

	if hasGenesis {
		for _, key := range genesis.members {
			atoms[key] = "table:" + genesis.table.name
		}
	}

	return atoms, nil
}
