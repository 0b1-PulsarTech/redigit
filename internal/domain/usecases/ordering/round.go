package ordering

import (
	"fmt"
	"maps"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// converge repeats step until a full round changes nothing, bounded by
// roundLimit. Convergence is guaranteed by construction (monotone, bounded
// above by limit); the guard exists so a bug never turns into a hang.
func (e *engine) converge(roundLimit int) (int, error) {
	for round := 1; round <= roundLimit; round++ {
		if !e.step() {
			return round, nil
		}
	}

	return roundLimit, fmt.Errorf(
		"%w: %w within %d rounds",
		apperr.ErrIncomplete, ErrNotConverged, roundLimit,
	)
}

// step runs one full round: the per-declaration pass, a global edge sweep
// that lets a dependent catch up to a provider the floor passes just moved,
// and a refresh of the unit floor for the next round. Every piece only ever
// raises an assignment, so a round either advances the fixed point or leaves
// it untouched.
func (e *engine) step() bool {
	changed := false

	for _, decl := range e.decls {
		if e.raise(decl.Key, e.bound(decl)) {
			changed = true
		}
	}

	if e.propagateEdges() {
		changed = true
	}

	if e.recomputeUnitFloor() {
		changed = true
	}

	return changed
}

// dangling reports an edge naming a key no declaration in the graph carries.
// Assuming these never occur was wrong: a rule pack can name a provider it
// inferred from a comment header or a table name that no declaration matches,
// and reading the missing side as position zero both invents a placement and
// pins whatever waits on it to the very start.
func (e *engine) dangling(key entities.DeclKey) bool {
	_, known := e.graph.Decl(key)

	return !known
}

// bound is the position a declaration cannot be born before: its base, every
// provider it needs plus delay, and the floor its unit carries.
func (e *engine) bound(decl entities.Decl) entities.Position {
	bound := e.base[decl.Key]

	for _, edge := range e.graph.Needs(decl.Key) {
		if e.dangling(edge.Provider) {
			continue
		}

		bound = max(bound, e.assign[edge.Provider]+e.delay(decl.Key, edge.Provider))
	}

	if floor, found := e.unitFloor[decl.Unit]; found {
		bound = max(bound, floor)
	}

	return bound
}

// delay is +1 when dependent is newly inserted and provider is not: an
// inserted commit is emitted before the original commit at that position, so
// depending on an original-history commit must wait one slot.
func (e *engine) delay(dependent, provider entities.DeclKey) entities.Position {
	if e.inserted[dependent] && !e.inserted[provider] {
		return 1
	}

	return 0
}

// raise assigns bound, capped at limit, when it is later than the current
// assignment. It never moves a declaration earlier, which is what keeps the
// whole fixed point monotone.
func (e *engine) raise(key entities.DeclKey, bound entities.Position) bool {
	capped := min(bound, e.limit)
	if capped <= e.assign[key] {
		return false
	}

	e.assign[key] = capped

	return true
}

// propagateEdges re-applies every edge's constraint after the per-declaration
// pass, so a dependent catches up within the same round to a provider a
// floor pass just pushed later. This is the spec's readersFloor pass: nothing
// is born before its first reader.
func (e *engine) propagateEdges() bool {
	changed := false

	for _, edge := range e.graph.Edges() {
		if e.dangling(edge.Dependent) || e.dangling(edge.Provider) {
			continue
		}

		if e.raise(edge.Dependent, e.assign[edge.Provider]+e.delay(edge.Dependent, edge.Provider)) {
			changed = true
		}
	}

	return changed
}

// recomputeUnitFloor rebuilds the per-unit floor from the earliest external
// consumer of any declaration in that unit. A unit is born when someone
// outside it needs it: counting a unit's own declarations here is the bug the
// prior work traced a 487-position gap to. It reports whether the floor
// itself changed, because the per-declaration pass above reads last round's
// floor: a floor that only just appeared has not raised anything yet. This is
// the spec's unitExternalFloor pass.
func (e *engine) recomputeUnitFloor() bool {
	floors := make(map[entities.UnitID]entities.Position, len(e.unitFloor))

	for _, decl := range e.decls {
		for _, edge := range e.graph.ExternalConsumers(decl.Key) {
			if e.dangling(edge.Dependent) {
				continue
			}

			// The reader's position is read back into the provider's frame by
			// removing the insertion delay. Without that, the floor pulls the
			// provider up to its reader while the delay pushes the reader one
			// past the provider, and the pair climbs to the limit forever:
			// an inserted file reading an existing one across a unit boundary
			// is ordinary, and it reported both as unresolvable.
			reader := max(e.assign[edge.Dependent]-e.delay(edge.Dependent, decl.Key), 0)
			if current, found := floors[decl.Unit]; !found || reader < current {
				floors[decl.Unit] = reader
			}
		}
	}

	changed := !maps.Equal(e.unitFloor, floors)
	e.unitFloor = floors

	return changed
}
