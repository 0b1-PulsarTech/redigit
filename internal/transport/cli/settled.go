package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
)

// settled runs the two stages every command needs. The ordering is returned
// even when it carries a complaint: a cycle or an unresolved declaration is
// what the caller asked to see, not a reason to print nothing.
func settled(ctx context.Context, pl pipeline) (entities.Graph, entities.Ordering, error) {
	result, err := pl.Graph(ctx)
	if err != nil {
		return entities.Graph{}, entities.Ordering{}, fmt.Errorf("build graph: %w", err)
	}

	warnBlind(result)

	graph := result.Graph

	order, err := pl.Order(graph)

	// A fixed point that never settled is fatal: it still hands back a full
	// map of positions, so every measure downstream reads as complete while
	// describing nothing but the seed. A cycle or an unresolved declaration
	// is different - the ordering settled, and the complaint is the finding.
	switch {
	case errors.Is(err, ordering.ErrNotConverged), order.At == nil:
		if err == nil {
			err = fmt.Errorf("%w: no positions", apperr.ErrIncomplete)
		}

		return entities.Graph{}, entities.Ordering{}, fmt.Errorf("settle order: %w", err)
	case order.Rounds < 1:
		return entities.Graph{}, entities.Ordering{}, fmt.Errorf(
			"settle order: %w: the fixed point ran %d rounds",
			apperr.ErrIncomplete, order.Rounds,
		)
	case err != nil:
		slog.Warn("the ordering settled with complaints", slog.String("detail", err.Error()))
	}

	return graph, order, nil
}

func encode(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}
