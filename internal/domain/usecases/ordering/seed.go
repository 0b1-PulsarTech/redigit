package ordering

import (
	"maps"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// seed computes each declaration's base position and whether it is newly
// inserted (its file carries no evidence in the source history), which is
// what delay branches on. Base is one floor among several, never a
// privileged seed: with no history at all every declaration seeds at 0.
func seed(
	decls []entities.Decl,
	snap entities.Snapshot,
) (base map[entities.DeclKey]entities.Position, inserted map[entities.DeclKey]bool) {
	base = make(map[entities.DeclKey]entities.Position, len(decls))
	inserted = make(map[entities.DeclKey]bool, len(decls))

	for _, decl := range decls {
		if !snap.HasHistory() {
			base[decl.Key] = 0

			continue
		}

		// simplification: base comes from the file's FIRST TOUCH, not from the
		// position that first carried this particular declaration. A file is
		// born with three declarations and gains eight over months, so this is
		// too early for all but the first few. It is a floor, so an early one
		// only loses chronological fidelity, never correctness: the edges still
		// forbid a declaration being born before what it needs. Closing the gap
		// needs Snapshot to carry, per position, which declarations existed -
		// which means rendering each historical version of each file.
		touch := snap.FirstTouch(decl.Key.File)
		if !touch.IsSet() {
			base[decl.Key] = 0
			inserted[decl.Key] = true

			continue
		}

		base[decl.Key] = touch
	}

	return base, inserted
}

func seedFloored(
	decls []entities.Decl,
	snap entities.Snapshot,
	floor map[entities.DeclKey]entities.Position,
) (base map[entities.DeclKey]entities.Position, inserted map[entities.DeclKey]bool) {
	if floor == nil {
		return seed(decls, snap)
	}

	base = make(map[entities.DeclKey]entities.Position, len(decls))
	inserted = make(map[entities.DeclKey]bool, len(decls))

	// A declaration the floor never saw (a statement piece of a function) seeds
	// as in history mode: marking it inserted pinned 185 of them at the limit.
	fallback, fallbackInserted := seed(decls, snap)

	for _, decl := range decls {
		if at, born := floor[decl.Key]; born {
			base[decl.Key] = at

			continue
		}

		base[decl.Key] = fallback[decl.Key]
		inserted[decl.Key] = fallbackInserted[decl.Key]
	}

	return base, inserted
}

func (eng *engine) reseed(snap entities.Snapshot, config Config) {
	eng.base, eng.inserted = seedFloored(eng.decls, snap, config.Floor)
	// newEngine built layers and anchors on the first-touch seeds: redo them.
	applyLayers(eng.decls, eng.base, config.Layers, config.Features)
	applyAnchors(eng.graph, eng.base)

	eng.assign = make(map[entities.DeclKey]entities.Position, len(eng.base))
	maps.Copy(eng.assign, eng.base)
}

// limitFor is the highest position the fixed point may assign. The order
// carries spare slots past the source history's end for tail commits that
// read each other back; the prior work used a slack of 8.
func limitFor(snap entities.Snapshot, slack int) entities.Position {
	var last entities.Position

	for _, touch := range snap.History {
		last = max(last, touch.Position)
	}

	return last + entities.Position(slack)
}
