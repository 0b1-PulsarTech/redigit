package passes

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

func slotIDs(draft *shaping.Draft) []int {
	slots := draft.Slots()
	ids := make([]int, 0, len(slots))

	for _, slot := range slots {
		ids = append(ids, slot.ID)
	}

	return ids
}

func place(draft *shaping.Draft, known origins, own int, held block) shaping.Moves {
	ownRank, _ := draft.Rank(own)

	after, found := latestProvider(draft, held.keys, ownRank)
	if !found {
		return nil
	}

	moves, _ := draft.Spawn(after, held.commit(), held.keys)
	known.inherit(moves, own)

	return moves
}

func (tidy Tidy) foreign(
	ctx context.Context, draft *shaping.Draft, known origins,
) (shaping.Moves, error) {
	var all shaping.Moves

	for _, id := range slotIDs(draft) {
		if err := ctx.Err(); err != nil {
			return all, fmt.Errorf("tidy foreign tests: %w", err)
		}

		// A block refused for a sibling in the same slot may fit once that sibling has moved,
		// so the slot repeats until quiet; each productive round empties a block.
		moves, err := fixpoint(
			blockCount(draft, id)+1,
			func() (shaping.Moves, error) { return tidy.foreignRound(draft, known, id), nil },
		)
		all = append(all, moves...)

		if err != nil {
			return all, err
		}
	}

	return all, nil
}

func blockCount(draft *shaping.Draft, id int) int {
	rank, live := draft.Rank(id)
	if !live {
		return 0
	}

	return len(blocksByDir(infosOf(draft.Graph(), draft.Slots()[rank].Commit.Decls)))
}

func (tidy Tidy) foreignRound(draft *shaping.Draft, known origins, id int) shaping.Moves {
	rank, live := draft.Rank(id)
	if !live {
		return nil
	}

	infos := infosOf(draft.Graph(), draft.Slots()[rank].Commit.Decls)

	home, found := primaryDir(infos)
	if !found {
		return nil
	}

	var all shaping.Moves

	for _, held := range blocksByDir(infos) {
		if held.dir == home || !tidy.worthMoving(held) {
			continue
		}

		all = append(all, place(draft, known, id, held)...)
	}

	return all
}

func (tidy Tidy) worthMoving(held block) bool {
	return held.tests >= tidy.cfg.MinTestLines && held.prod <= tidy.cfg.MaxProdLines
}
