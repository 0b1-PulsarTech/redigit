package session

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/profiling"
)

type timedPipeline struct {
	inner   Pipeline
	timings *profiling.Timings
}

var _ Pipeline = timedPipeline{}

func TimedStart(inner Starter, timings *profiling.Timings) Starter {
	return func(ctx context.Context, cfg bootstrap.Config) (Pipeline, error) {
		defer timings.Measure("start")()

		pl, err := inner(ctx, cfg)
		if err != nil {
			return nil, err
		}

		return timedPipeline{inner: pl, timings: timings}, nil
	}
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (timed timedPipeline) Births(ctx context.Context) (indexing.Births, error) {
	defer timed.timings.Measure("births")()

	return timed.inner.Births(ctx)
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (timed timedPipeline) Graph(ctx context.Context) (indexing.Result, error) {
	defer timed.timings.Measure("graph")()

	return timed.inner.Graph(ctx)
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (timed timedPipeline) Order(graph entities.Graph) (entities.Ordering, error) {
	defer timed.timings.Measure("order")()

	return timed.inner.Order(graph)
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (timed timedPipeline) Plan(
	graph entities.Graph,
	order entities.Ordering,
) (entities.Plan, error) {
	defer timed.timings.Measure("plan")()

	return timed.inner.Plan(graph, order)
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (timed timedPipeline) Audit(
	graph entities.Graph,
	order entities.Ordering,
) (auditing.Result, error) {
	defer timed.timings.Measure("audit")()

	return timed.inner.Audit(graph, order)
}

func (timed timedPipeline) Unseen() []auditing.Unseen { return timed.inner.Unseen() }

func (timed timedPipeline) Unclaimed() []auditing.Unclaimed { return timed.inner.Unclaimed() }

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (timed timedPipeline) Emit(
	ctx context.Context,
	plan entities.Plan,
	graph entities.Graph,
	sink emitting.Sink,
) ([]emitting.Commit, error) {
	defer timed.timings.Measure("emit")()

	return timed.inner.Emit(ctx, plan, graph, sink)
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (timed timedPipeline) Close(ctx context.Context) error {
	defer timed.timings.Measure("close")()

	return timed.inner.Close(ctx)
}
