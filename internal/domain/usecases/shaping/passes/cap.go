package passes

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

const CapName = "finish-cap"

type (
	CapConfig struct {
		MaxAdded int `json:"maxAdded"`
	}

	Cap struct{ cfg CapConfig }
)

func NewCap(raw []byte) (Cap, error) {
	const defaultMaxAdded = 850

	spec := struct {
		CapConfig

		Type string `json:"type"`
	}{CapConfig: CapConfig{MaxAdded: defaultMaxAdded}}

	if err := shaping.DecodeStrict(raw, &spec); err != nil {
		return Cap{}, fmt.Errorf("%s: %w", CapName, err)
	}

	if spec.MaxAdded < 1 {
		return Cap{}, fmt.Errorf(
			"%w: %s maxAdded must be at least 1",
			shaping.ErrMalformed,
			CapName,
		)
	}

	return Cap{cfg: spec.CapConfig}, nil
}

func (pass Cap) Name() string { return CapName }

func (pass Cap) Apply(ctx context.Context, draft *shaping.Draft) (shaping.Moves, error) {
	// A spawned row can pass the cap too, and the decl count bounds the spawns.
	moves, err := fixpoint(heldCount(draft)+1, func() (shaping.Moves, error) {
		var all shaping.Moves

		for _, id := range slotIDs(draft) {
			if err := ctx.Err(); err != nil {
				return all, fmt.Errorf("%s: %w", CapName, err)
			}

			all = append(all, pass.shrink(draft, id)...)
		}

		return all, nil
	})

	retitle(draft, moves)
	pass.report(ctx, draft)

	return moves, err
}

func retitle(draft *shaping.Draft, moves shaping.Moves) {
	changed := map[int]bool{}

	for _, move := range moves {
		changed[move.From], changed[move.To] = true, true
	}

	for _, slot := range draft.Slots() {
		isDraft := entities.IsDraft(slot.Commit.Subject)
		if changed[slot.ID] && len(slot.Commit.Decls) > 0 && (isDraft || slot.Added) {
			draft.Retitle(slot.ID, titleOf(draft.Graph(), slot.Commit))
		}
	}
}

func (pass Cap) report(ctx context.Context, draft *shaping.Draft) {
	born := firstRows(draft)

	var position int

	for rank, slot := range draft.Slots() {
		if len(slot.Commit.Decls) == 0 {
			continue
		}

		if lines := addedLines(draft, slot.Commit.Decls, born, rank); lines > pass.cfg.MaxAdded {
			slog.WarnContext(ctx, CapName+": a row stays past the cap, no test file can leave",
				slog.Int("position", position), slog.Int("lines", lines),
				slog.Int("cap", pass.cfg.MaxAdded))
		}

		position++
	}
}

func (pass Cap) shrink(draft *shaping.Draft, id int) shaping.Moves {
	rank, live := draft.Rank(id)
	if !live {
		return nil
	}

	source := draft.Slots()[rank].Commit
	born := firstRows(draft)

	if addedLines(draft, source.Decls, born, rank) <= pass.cfg.MaxAdded {
		return nil
	}

	var leaving []entities.DeclKey

	for _, file := range filesByHandOrder(draft.Graph(), source.Decls) {
		joined := withConsumers(draft, source.Decls, append(slices.Clone(leaving), file.keys...))

		whole := len(joined) == len(source.Decls)
		if whole || !allTest(infosOf(draft.Graph(), joined)) || !draft.Fits(joined, id) {
			continue
		}

		leaving = joined

		staying := slices.DeleteFunc(slices.Clone(source.Decls), func(key entities.DeclKey) bool {
			return slices.Contains(leaving, key)
		})
		if addedLines(draft, staying, born, rank) <= pass.cfg.MaxAdded {
			break
		}
	}

	if len(leaving) == 0 {
		return nil
	}

	moves, _ := draft.Spawn(id, handedOff(draft.Graph(), source, leaving), leaving)

	return moves
}

func firstRows(draft *shaping.Draft) map[string]int {
	born := map[string]int{}

	for rank, slot := range slices.Backward(draft.Slots()) {
		for _, key := range slot.Commit.Decls {
			born[key.File] = rank
		}
	}

	return born
}

func addedLines(draft *shaping.Draft, keys []entities.DeclKey, born map[string]int, rank int) int {
	var added int

	framed := map[string]bool{}

	for _, key := range keys {
		decl, _ := draft.Graph().Decl(key)
		added += decl.Lines

		// The file's frame is only ever added by the row that bears the file.
		if !framed[key.File] && born[key.File] == rank {
			added += decl.Frame
			framed[key.File] = true
		}
	}

	return added
}
