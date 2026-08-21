package ordering

import (
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// engine is the working state of one Order call: the graph and snapshot
// evidence it was built from, plus the maps the fixed point mutates round by
// round. It never outlives the call that built it.
type engine struct {
	graph     entities.Graph
	decls     []entities.Decl
	base      map[entities.DeclKey]entities.Position
	inserted  map[entities.DeclKey]bool
	assign    map[entities.DeclKey]entities.Position
	unitFloor map[entities.UnitID]entities.Position
	limit     entities.Position
}

func newEngine(graph entities.Graph, snap entities.Snapshot, slack int) *engine {
	decls := graph.Decls()
	slices.SortFunc(decls, func(a, b entities.Decl) int { return compareKeys(a.Key, b.Key) })

	base, inserted := seed(decls, snap)
	applyAnchors(graph, base)

	assign := make(map[entities.DeclKey]entities.Position, len(base))
	maps.Copy(assign, base)

	return &engine{
		graph:     graph,
		decls:     decls,
		base:      base,
		inserted:  inserted,
		assign:    assign,
		unitFloor: map[entities.UnitID]entities.Position{},
		limit:     limitFor(snap, slack),
	}
}

// ordering builds the entities.Ordering the caller sees from the engine's
// current assignment.
func (e *engine) ordering(rounds int, cycles []entities.Cycle) entities.Ordering {
	at := make(map[entities.DeclKey]entities.Position, len(e.assign))
	maps.Copy(at, e.assign)

	return entities.Ordering{At: at, Cycles: cycles, Rounds: rounds}
}

// unresolved reports the declarations whose true bound, recomputed against
// the converged assignment, still exceeds limit. Bound is monotone
// non-decreasing round over round, so at a fixed point this recomputation
// equals the highest bound ever demanded of that declaration: still pinned at
// limit is a real unresolved dependency, not a warning.
func (e *engine) unresolved() []entities.DeclKey {
	var out []entities.DeclKey

	for _, decl := range e.decls {
		if e.bound(decl) > e.limit {
			out = append(out, decl.Key)
		}
	}

	slices.SortFunc(out, compareKeys)

	return out
}
