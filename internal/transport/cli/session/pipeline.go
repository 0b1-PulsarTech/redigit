package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=pipeline.go -destination=pipeline_mock_test.go -package=session

type Pipeline interface {
	Births(ctx context.Context) (indexing.Births, error)
	Graph(ctx context.Context) (indexing.Result, error)
	Order(graph entities.Graph) (entities.Ordering, error)
	Plan(graph entities.Graph, order entities.Ordering) (entities.Plan, error)
	Audit(graph entities.Graph, order entities.Ordering) (auditing.Result, error)
	Unseen() []auditing.Unseen
	Unclaimed() []auditing.Unclaimed
	Emit(
		ctx context.Context,
		plan entities.Plan,
		graph entities.Graph,
		sink emitting.Sink,
	) ([]emitting.Commit, error)
	Close(ctx context.Context) error
}

var _ Pipeline = (*bootstrap.Pipeline)(nil)

type Starter func(ctx context.Context, cfg bootstrap.Config) (Pipeline, error)

func WithPipeline(
	ctx context.Context,
	cfg bootstrap.Config,
	start Starter,
	run func(Pipeline) error,
) error {
	pl, err := start(ctx, cfg)
	if errors.Is(err, bootstrap.ErrConfig) {
		return err
	}

	if err != nil {
		return fmt.Errorf("start pipeline: %w", err)
	}

	defer func() { WarnClosed(pl.Close(context.WithoutCancel(ctx))) }()

	WarnUnseen(pl.Unseen())
	WarnUnclaimed(pl.Unclaimed())

	return run(pl)
}
