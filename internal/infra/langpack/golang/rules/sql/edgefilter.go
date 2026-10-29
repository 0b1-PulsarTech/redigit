package sql

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func DropConstruction(snap entities.Snapshot, edges []entities.Edge) []entities.Edge {
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

		// Generated code calls no method and waits on no consumer; New/WithTx name Queries, bound to
		// interfaces elsewhere, so without this every closure held every table.
		if structural && isGenerated(edge.Dependent.File) {
			continue
		}

		kept = append(kept, edge)
	}

	return kept
}
