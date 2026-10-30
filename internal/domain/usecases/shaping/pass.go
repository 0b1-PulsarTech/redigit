package shaping

import (
	"context"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	Pass interface {
		Name() string
		Apply(ctx context.Context, draft *Draft) (Moves, error)
	}

	Move struct {
		Decl     entities.DeclKey
		From, To int
		Reseat   bool
	}

	Moves []Move
)

func (draft *Draft) Spawn(
	after int,
	commit entities.PlannedCommit,
	keys []entities.DeclKey,
) (Moves, bool) {
	// Nothing is touched unless every key is placed and the whole set fits.
	if len(keys) == 0 || !draft.Fits(keys, after) {
		return nil, false
	}

	for _, key := range keys {
		if _, placed := draft.Of(key); !placed {
			return nil, false
		}
	}

	at := 0

	if after != NoSlot {
		rank, _ := draft.Rank(after)
		at = rank + 1
	}

	commit.Decls = nil
	id := draft.next
	draft.next++
	draft.slots = slices.Insert(draft.slots, at, Slot{ID: id, Commit: commit, Added: true})
	draft.reindex()

	moves := make(Moves, 0, len(keys))
	for _, key := range keys {
		moves = append(moves, draft.relocate(key, id))
	}

	return moves, true
}

func (draft *Draft) Merge(from, into int) (Moves, bool) {
	// The old id stays an alias so a pass holding it still finds the decls.
	from, into = draft.at.Resolve(from), draft.at.Resolve(into)

	_, hasFrom := draft.ranks[from]
	_, hasInto := draft.ranks[into]

	if from == into || !hasFrom || !hasInto {
		return nil, false
	}

	held := slices.Clone(draft.slots[draft.ranks[from]].Commit.Decls)
	moves := make(Moves, 0, len(held))

	for _, key := range held {
		moves = append(moves, draft.relocate(key, into))
	}

	draft.at.Merge(from, into)
	draft.slots = slices.Delete(draft.slots, draft.ranks[from], draft.ranks[from]+1)
	draft.reindex()

	return moves, true
}

func (draft *Draft) relocate(key entities.DeclKey, to int) Move {
	from, _ := draft.at.Slot(key)

	source := &draft.slots[draft.ranks[from]].Commit
	source.Decls = slices.DeleteFunc(
		source.Decls, func(held entities.DeclKey) bool { return held == key },
	)

	target := &draft.slots[draft.ranks[to]].Commit
	target.Decls = append(target.Decls, key)
	draft.at.Place(key, to)

	return Move{Decl: key, From: from, To: to}
}
