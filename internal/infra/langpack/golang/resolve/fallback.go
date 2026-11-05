package resolve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync/atomic"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

var ErrSecondaryStart = errors.New("start secondary resolver")

//go:generate go tool -modfile=../../../../../tools/go.mod mockgen -source=fallback.go -destination=fallback_mock_test.go -package=resolve

type (
	Secondary interface {
		indexing.Resolver
		Close(ctx context.Context) error
	}

	stopper interface {
		Close(ctx context.Context) error
	}

	adhocReporter interface{ AdhocErr() error }

	hiddenReporter interface{ HiddenLost() int64 }

	Fallback struct {
		primary   indexing.Resolver
		secondary Secondary
		logger    *slog.Logger
		count     atomic.Int64
		failure   atomic.Pointer[error]
	}
)

var _ indexing.Resolver = (*Fallback)(nil)

func NewFallback(primary indexing.Resolver, secondary Secondary, logger *slog.Logger) *Fallback {
	return &Fallback{primary: primary, secondary: secondary, logger: logger}
}

func (fallback *Fallback) Fallbacks() int64 { return fallback.count.Load() }

func (fallback *Fallback) Err() error {
	if failed := fallback.failure.Load(); failed != nil {
		return *failed
	}

	return nil
}

func (resolver *Typed) AdhocErr() error {
	if failed := resolver.adhocErr.Load(); failed != nil {
		return *failed
	}

	return nil
}

func (resolver *Typed) HiddenLost() int64 { return resolver.hiddenLost.Load() }

func lostRows(groups []index) int64 {
	lost := map[row]bool{}
	for _, group := range groups {
		maps.Copy(lost, group.lost)
	}

	return int64(len(lost))
}

func (fallback *Fallback) References(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	return ask(
		fallback,
		func() ([]entities.Location, error) { return fallback.primary.References(ctx, at) },
		func() ([]entities.Location, error) { return fallback.secondary.References(ctx, at) },
	)
}

func (fallback *Fallback) Implementations(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	return ask(
		fallback,
		func() ([]entities.Location, error) { return fallback.primary.Implementations(ctx, at) },
		func() ([]entities.Location, error) { return fallback.secondary.Implementations(ctx, at) },
	)
}

func (fallback *Fallback) Definition(
	ctx context.Context,
	at entities.Location,
) (entities.Location, error) {
	return ask(
		fallback,
		func() (entities.Location, error) { return fallback.primary.Definition(ctx, at) },
		func() (entities.Location, error) { return fallback.secondary.Definition(ctx, at) },
	)
}

func (fallback *Fallback) Close(ctx context.Context) error {
	level := slog.LevelInfo
	if fallback.count.Load() > 0 {
		level = slog.LevelWarn
	}

	fallback.logger.Log(
		ctx, level, "typed resolver fallbacks", slog.Int64("count", fallback.count.Load()),
	)

	closeErr := fallback.settlePrimary(ctx)

	if err := fallback.secondary.Close(ctx); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("close secondary resolver: %w", err))
	}

	return closeErr
}

func (fallback *Fallback) settlePrimary(ctx context.Context) error {
	var stopErr error

	if stopping, ok := fallback.primary.(stopper); ok {
		if err := stopping.Close(ctx); err != nil {
			stopErr = fmt.Errorf("close primary resolver: %w", err)
		}
	}

	// The ad hoc outcome is final only once the primary's load has finished or been cancelled.
	if reporting, ok := fallback.primary.(adhocReporter); ok {
		if failed := reporting.AdhocErr(); failed != nil {
			fallback.logger.Warn(
				"typed resolver skipped ad hoc files", slog.String("error", failed.Error()),
			)
		}
	}

	if reporting, ok := fallback.primary.(hiddenReporter); ok && reporting.HiddenLost() > 0 {
		fallback.logger.Warn(
			"hidden files read packages their pass met only as export data, "+
				"so any implements edge to those is missing",
			slog.Int64("count", reporting.HiddenLost()),
		)
	}

	return stopErr
}

func ask[Found any](
	fallback *Fallback,
	primary, secondary func() (Found, error),
) (Found, error) {
	found, err := primary()
	if !errors.Is(err, ErrNotCovered) {
		return found, err
	}

	fallback.count.Add(1)

	found, err = secondary()

	// The indexer files a failed query as a blind spot and goes on, so the start failure is kept
	// here for the pipeline to refuse the run.
	if errors.Is(err, ErrSecondaryStart) {
		fallback.failure.CompareAndSwap(nil, &err)
	}

	return found, err
}
