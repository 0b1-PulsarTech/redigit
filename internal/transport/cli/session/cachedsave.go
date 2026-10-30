package session

import (
	"errors"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/infra/plancache"
)

func (cached *cachedPipeline) save() {
	// Complete with the plan too, so any later command is answered from the entry.
	if !cached.hasGraph || !cached.hasOrder {
		return
	}

	if !cached.hasPlan {
		plan, err := cached.live.Plan(cached.result.Graph, cached.entry.Ordering)
		if err != nil {
			slog.Warn(
				"the plan cache was not saved: planning failed",
				slog.String("error", err.Error()),
			)

			return
		}

		cached.entry.Plan = plan
	}

	entry := plancache.Pack(cached.result)
	entry.Commit = cached.commit
	entry.Ordering, entry.OrderNote, entry.Plan = cached.entry.Ordering, cached.entry.OrderNote, cached.entry.Plan
	entry.Unseen, entry.Unclaimed = cached.live.Unseen(), cached.live.Unclaimed()

	switch err := cached.store.Save(cached.key, entry); {
	case errors.Is(err, plancache.ErrTooLarge):
		slog.Warn("the plan cache was not saved: this plan is too large to keep")
	case err != nil:
		slog.Warn("the plan cache was not saved", slog.String("error", err.Error()))
	}
}
