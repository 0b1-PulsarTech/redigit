package sql

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// DropConstruction removes the structural edges whose dependent is sqlc
// output. New and WithTx name Queries, a type bound to interfaces elsewhere,
// so the bound-type rules tied each of them to all 606 methods, and Queries
// itself to the consumer ports it satisfies (a leadsense interface, whose
// module then waited on the database module): every closure held every table
// and none could be born with its first caller. Generated code calls no
// method and waits for no consumer.
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

		if structural && isGenerated(edge.Dependent.File) {
			continue
		}

		kept = append(kept, edge)
	}

	return kept
}
