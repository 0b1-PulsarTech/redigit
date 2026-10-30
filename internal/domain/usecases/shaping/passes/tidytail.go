package passes

import (
	"context"
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

var ErrUnsettled = errors.New("rounds still moving")

type origins map[int]int

func originsOf(draft *shaping.Draft) origins {
	known := make(origins, len(draft.Slots()))
	for rank, slot := range draft.Slots() {
		known[slot.ID] = rank
	}

	return known
}

func (known origins) inherit(moves shaping.Moves, source int) {
	// A piece spawned from a slot takes that slot's place in the original plan order.
	if len(moves) > 0 {
		known[moves[0].To] = known[source]
	}
}

func (known origins) before(slot, own int) bool {
	return known[slot] < known[own] || (known[slot] == known[own] && slot < own)
}

func (tidy Tidy) tail(
	ctx context.Context, draft *shaping.Draft, known origins,
) (shaping.Moves, error) {
	rounds := heldCount(draft) + 1

	// A move shifts the window, so rounds repeat. Keyed by order (non-test slots infinity), the
	// live sequence falls lexicographically at each reseat; spawns are bounded by the block
	// count, so the decl count bounds the rounds.
	return fixpoint(rounds, func() (shaping.Moves, error) {
		return tidy.tailRound(ctx, draft, known)
	})
}

func heldCount(draft *shaping.Draft) int {
	var held int

	for _, slot := range draft.Slots() {
		held += len(slot.Commit.Decls)
	}

	return held
}

func fixpoint(rounds int, round func() (shaping.Moves, error)) (shaping.Moves, error) {
	var all shaping.Moves

	for range rounds {
		moves, err := round()
		all = append(all, moves...)

		if err != nil || len(moves) == 0 {
			return all, err
		}
	}

	return all, fmt.Errorf("tidy: %w", ErrUnsettled)
}

func tailIDs(draft *shaping.Draft, window int) []int {
	var ids []int

	for _, slot := range draft.Slots() {
		if len(slot.Commit.Decls) > 0 {
			ids = append(ids, slot.ID)
		}
	}

	return ids[max(0, len(ids)-window):]
}

func (tidy Tidy) tailRound(
	ctx context.Context, draft *shaping.Draft, known origins,
) (shaping.Moves, error) {
	var all shaping.Moves

	for _, id := range tailIDs(draft, tidy.cfg.TailWindow) {
		if err := ctx.Err(); err != nil {
			return all, fmt.Errorf("tidy trailing tests: %w", err)
		}

		rank, live := draft.Rank(id)
		if !live {
			continue
		}

		infos := infosOf(draft.Graph(), draft.Slots()[rank].Commit.Decls)
		if len(infos) == 0 || !allTest(infos) {
			continue
		}

		blocks := blocksByDir(infos)
		for _, held := range blocks {
			after, wanted := settledAfter(draft, known, id, held)
			if !wanted {
				continue
			}

			if len(blocks) == 1 {
				// The whole commit travels as it is, so its subject stays.
				moves, _ := draft.Reseat(id, after)
				all = append(all, moves...)

				continue
			}

			// A multi-block all-test commit has no single subject to keep, so each
			// block leaves as its own DRAFT piece.
			moves, _ := draft.Spawn(after, held.commit(), held.keys)
			known.inherit(moves, id)

			all = append(all, moves...)
		}
	}

	return all, nil
}

func settledAfter(draft *shaping.Draft, known origins, own int, held block) (int, bool) {
	ownRank, _ := draft.Rank(own)

	provider, found := latestProvider(draft, held.keys, ownRank-1)
	if !found {
		return 0, false
	}

	providerRank, _ := draft.Rank(provider)
	after, inRun := provider, true

	// Placed means only test-only live slots sit between provider and block; emptied slots
	// do not count. Otherwise it goes after the test run following the provider, ahead of any test
	// commit ordered after its own in the original plan (a piece takes its source's place there).
	for _, slot := range draft.Slots()[providerRank+1 : ownRank] {
		if len(slot.Commit.Decls) == 0 {
			continue
		}

		inRun = inRun && known.before(slot.ID, own) &&
			allTest(infosOf(draft.Graph(), slot.Commit.Decls))
		if inRun {
			after = slot.ID
		}
	}

	return after, !inRun
}

func allTest(infos []declInfo) bool {
	for _, info := range infos {
		if !info.test {
			return false
		}
	}

	return true
}
