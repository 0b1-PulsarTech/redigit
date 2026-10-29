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

		if structural && isGenerated(edge.Dependent.File) {
			continue
		}

		kept = append(kept, edge)
	}

	return kept
}
