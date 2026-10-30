package session

import (
	"context"
	"errors"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/infra/plancache"
)

type cachedPipeline struct {
	launch   func() (Pipeline, error)
	live     Pipeline
	store    plancache.Store
	key      string
	commit   string
	measure  func(stage string) func()
	isHit    bool
	result   indexing.Result
	entry    plancache.Entry
	hasGraph bool
	hasOrder bool
	hasPlan  bool
}

var _ Pipeline = (*cachedPipeline)(nil)

func CachingStart(
	inner Starter,
	store plancache.Store,
	resolved Resolved,
	measure func(stage string) func(),
) Starter {
	if measure == nil {
		measure = func(string) func() { return func() {} }
	}

	return func(ctx context.Context, cfg bootstrap.Config) (Pipeline, error) {
		// A hit starts nothing; a miss runs the key's commit and saves on close.
		pinned := cfg
		pinned.Rev = resolved.Commit
		launch := func() (Pipeline, error) { return inner(ctx, pinned) }

		entry, err := store.Load(resolved.Key)
		if err == nil && entry.Commit != resolved.Commit {
			err = plancache.ErrCorrupt
		}

		if err == nil {
			return &cachedPipeline{launch: launch, isHit: true, entry: entry}, nil
		}

		if !errors.Is(err, plancache.ErrMiss) {
			slog.Warn("ignoring an unreadable plan cache entry", slog.String("error", err.Error()))
		}

		live, err := launch()
		if err != nil {
			return nil, err
		}

		return &cachedPipeline{
			launch:  launch,
			live:    live,
			store:   store,
			key:     resolved.Key,
			commit:  resolved.Commit,
			measure: measure,
		}, nil
	}
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (cached *cachedPipeline) Graph(ctx context.Context) (indexing.Result, error) {
	if cached.isHit {
		return cached.entry.Result(), nil
	}

	result, err := cached.live.Graph(ctx)
	if err == nil {
		cached.result, cached.hasGraph = result, true
	}

	return result, err
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (cached *cachedPipeline) Order(graph entities.Graph) (entities.Ordering, error) {
	if cached.isHit {
		return cached.entry.Ordering, noteError(cached.entry.OrderNote)
	}

	order, err := cached.live.Order(graph)
	// Only a converged ordering is saved; a run that never converged is redone.
	if order.At != nil && order.Rounds >= 1 && !errors.Is(err, ordering.ErrNotConverged) {
		cached.entry.Ordering, cached.hasOrder = order, true

		if err != nil {
			cached.entry.OrderNote = err.Error()
		}
	}

	return order, err
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (cached *cachedPipeline) Plan(
	graph entities.Graph,
	order entities.Ordering,
) (entities.Plan, error) {
	if cached.isHit {
		return cached.entry.Plan, nil
	}

	plan, err := cached.live.Plan(graph, order)
	if err == nil {
		cached.entry.Plan, cached.hasPlan = plan, true
	}

	return plan, err
}

//nolint:wrapcheck // A decorator hands the inner stage error through unchanged.
func (cached *cachedPipeline) Close(ctx context.Context) error {
	if !cached.isHit {
		stop := cached.measure("save")
		cached.save()
		stop()
	}

	if cached.live == nil {
		return nil
	}

	return cached.live.Close(ctx)
}

//nolint:ireturn // The pipeline port is the product, as the real starter returns it.
func (cached *cachedPipeline) started() (Pipeline, error) {
	// A hit runs without a pipeline until births, audit or emit needs one.
	if cached.live == nil {
		live, err := cached.launch()
		if err != nil {
			return nil, err
		}

		cached.live = live
	}

	return cached.live, nil
}

func noteError(note string) error {
	if note == "" {
		return nil
	}

	return errors.New(note)
}
