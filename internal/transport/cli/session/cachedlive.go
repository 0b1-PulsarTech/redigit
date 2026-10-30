package session

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (cached *cachedPipeline) Births(ctx context.Context) (indexing.Births, error) {
	live, err := cached.started()
	if err != nil {
		return indexing.Births{}, err
	}

	return live.Births(ctx)
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (cached *cachedPipeline) Audit(
	graph entities.Graph,
	order entities.Ordering,
) (auditing.Result, error) {
	live, err := cached.started()
	if err != nil {
		return auditing.Result{}, err
	}

	return live.Audit(graph, order)
}

func (cached *cachedPipeline) Unseen() []auditing.Unseen {
	if cached.isHit {
		return cached.entry.Unseen
	}

	return cached.live.Unseen()
}

func (cached *cachedPipeline) Unclaimed() []auditing.Unclaimed {
	if cached.isHit {
		return cached.entry.Unclaimed
	}

	return cached.live.Unclaimed()
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (cached *cachedPipeline) Emit(
	ctx context.Context,
	plan entities.Plan,
	graph entities.Graph,
	sink emitting.Sink,
) ([]emitting.Commit, error) {
	live, err := cached.started()
	if err != nil {
		return nil, err
	}

	return live.Emit(ctx, plan, graph, sink)
}
