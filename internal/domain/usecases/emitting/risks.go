package emitting

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// risksFor reports, for every declaration born in planned, every reference it
// needs that is born strictly LATER than planned itself. A target born in
// this same commit or an earlier one is satisfied and is not a risk - that is
// the whole point of a vertical slice.
//
// A needed declaration this stage cannot find a position for is left out.
// Emit's checkCoverage already refuses to run at all when a declaration the
// GRAPH knows about landed in no commit, so reaching this branch means the
// target is not in the graph either - an edge to something outside what this
// run indexed - and this stage has nothing truthful to say about when it is
// born.
func risksFor(
	planned entities.PlannedCommit,
	graph entities.Graph,
	positions map[entities.DeclKey]entities.Position,
) []Risk {
	var risks []Risk

	for _, key := range planned.Decls {
		seen := map[entities.DeclKey]bool{}

		for _, edge := range graph.Needs(key) {
			target := edge.Provider
			if seen[target] || entities.FillsInterface(graph, edge) {
				continue
			}

			bornAt, known := positions[target]
			if !known || bornAt <= planned.Position {
				continue
			}

			seen[target] = true

			risks = append(risks, Risk{Chunk: key, Target: target, BornAt: bornAt})
		}
	}

	slices.SortFunc(risks, func(a, b Risk) int {
		return cmp.Or(
			entities.CompareKeys(a.Chunk, b.Chunk),
			entities.CompareKeys(a.Target, b.Target),
		)
	})

	return risks
}
