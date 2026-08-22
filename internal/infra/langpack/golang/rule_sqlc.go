package golang

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// sqlcRule links a generated dbgen/*.sql.go function to the "-- name: Name"
// query it came from in database/queries/*.sql. The generated file is Go,
// the query is SQL text with no Go declaration of its own, so the query is
// represented as a KindOther node: its file and the query name are all the
// identity it has.
type sqlcRule struct{}

func (sqlcRule) Name() string { return "sqlc" }

func (sqlcRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	byFile := groupByFile(decls)

	var edges []entities.Edge

	for _, qf := range queryFiles(snap.Files) {
		source, err := readSource(snap, qf)
		if err != nil {
			return nil, err
		}

		for _, name := range extractQueryNames(source) {
			queryKey := entities.DeclKey{File: qf, Kind: entities.KindOther, Name: name}

			for file, candidates := range byFile {
				if !isDBGenFile(file) {
					continue
				}

				if generated, found := findDecl(candidates, file, entities.KindFunc, name); found {
					edges = append(edges, entities.Edge{
						Dependent: generated,
						Provider:  queryKey,
						Reason:    entities.ReasonQuery,
					})
				}
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
