package idiom

import (
	"log/slog"
	"sync/atomic"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

var _ indexing.EdgeFilter = Pack{}

func (pack Pack) WithLogger(logger *slog.Logger) Pack {
	pack.logger, pack.reported = logger, &atomic.Int64{}

	return pack
}

func (pack Pack) FilterEdges(_ entities.Snapshot, edges []entities.Edge) []entities.Edge {
	kept, dropped := dropClosing(edges)
	pack.report(dropped)

	return kept
}

func (pack Pack) report(dropped int) {
	// The filter runs once per index pass and a cut can change what it drops: say so when it does.
	if pack.logger == nil || pack.reported.Swap(int64(dropped)) == int64(dropped) {
		return
	}

	if dropped == 0 {
		pack.logger.Warn("idioms: no edge dropped on the latest pass")

		return
	}

	pack.logger.Warn("idioms: edges dropped for closing a cycle", slog.Int("droppedEdges", dropped))
}

func isDI(reason entities.EdgeReason) bool {
	return reason == entities.ReasonProvides || reason == entities.ReasonRegistered
}

func dropClosing(edges []entities.Edge) ([]entities.Edge, int) {
	guard, dropped := newGuard(edges), 0
	if guard == nil {
		return edges, 0
	}

	kept := make([]entities.Edge, 0, len(edges))

	for _, edge := range edges {
		if isDI(edge.Reason) && !guard.accepts(edge) {
			dropped++

			continue
		}

		kept = append(kept, edge)
	}

	return kept, dropped
}
