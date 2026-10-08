package golang

import (
	"path"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// dropSqlcConstruction removes the edges that make sqlc's own db.go wait for
// every query method. New and WithTx name Queries, a type bound to an
// interface elsewhere, so the bound-type rules tied each of them to all 606
// methods; the closure of any repository then held every query and every
// table, and none could be born with its first caller. Building the struct
// calls no method.
func dropSqlcConstruction(snap entities.Snapshot, edges []entities.Edge) []entities.Edge {
	generated := map[string]bool{}
	isGenerated := func(file string) bool {
		known, seen := generated[file]
		if !seen {
			known = isDBGenOutput(snap, file)
			generated[file] = known
		}

		return known
	}

	kept := edges[:0:0]

	for _, edge := range edges {
		structural := edge.Reason == entities.ReasonImplements ||
			edge.Reason == entities.ReasonMethodSet
		sameOutput := path.Dir(edge.Dependent.File) == path.Dir(edge.Provider.File)

		if structural && sameOutput && isGenerated(edge.Dependent.File) &&
			isGenerated(edge.Provider.File) {
			continue
		}

		kept = append(kept, edge)
	}

	return kept
}
