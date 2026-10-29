package golang

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// sqlcRule links what sqlc generates to what it generated from. A query
// function waits for the "-- name:" block it came from, the block for every
// table it reads, and a table's struct for the generated package root. Linked
// by directory, every query landed hundreds of commits before the table it
// reads, and a function was born with no caller.
type sqlcRule struct{}

func (sqlcRule) Name() string { return "sqlc" }

func (sqlcRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	model, err := loadSQLModel(snap, decls)
	if err != nil {
		return nil, err
	}

	var edges []entities.Edge

	for _, query := range model.queries {
		for _, name := range query.tables {
			if create, created := model.tables[name].create(); created {
				edges = append(edges, entities.Edge{
					Dependent: query.block, Provider: create, Reason: entities.ReasonQuery,
				})
			}
		}

		for _, extension := range query.extensions {
			edges = append(edges, entities.Edge{
				Dependent: query.block, Provider: extension, Reason: entities.ReasonQuery,
			})
		}

		if query.method != (entities.DeclKey{}) {
			edges = append(edges, entities.Edge{
				Dependent: query.method, Provider: query.block, Reason: entities.ReasonQuery,
			})
		}
	}

	for _, name := range model.order {
		for _, root := range model.dbInit {
			if table := model.tables[name]; table.model != (entities.DeclKey{}) {
				edges = append(edges, entities.Edge{
					Dependent: table.model, Provider: root, Reason: entities.ReasonQuery,
				})
			}
		}
	}

	return edges, nil
}

func (sqlcRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
