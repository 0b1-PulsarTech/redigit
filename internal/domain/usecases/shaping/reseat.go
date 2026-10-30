package shaping

import "slices"

func (draft *Draft) Reseat(id, after int) (Moves, bool) {
	id, after = draft.at.Resolve(id), draft.at.Resolve(after)

	from, hasFrom := draft.ranks[id]
	target, hasTarget := NoSlot, after == NoSlot

	if !hasTarget {
		target, hasTarget = draft.ranks[after]
	}

	// A slot already right after its target has nowhere to go; NoSlot targets the front.
	if id == after || !hasFrom || !hasTarget || from == target+1 {
		return nil, false
	}

	slot := draft.slots[from]
	if len(slot.Commit.Decls) == 0 || !draft.Fits(slot.Commit.Decls, after) {
		return nil, false
	}

	draft.slots = slices.Delete(draft.slots, from, from+1)
	draft.reindex()

	at := 0
	if after != NoSlot {
		at = draft.ranks[after] + 1
	}

	draft.slots = slices.Insert(draft.slots, at, slot)
	draft.reindex()

	moves := make(Moves, 0, len(slot.Commit.Decls))
	for _, key := range slot.Commit.Decls {
		moves = append(moves, Move{Decl: key, From: id, To: after, Reseat: true})
	}

	return moves, true
}
