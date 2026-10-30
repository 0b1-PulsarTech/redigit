package session

import (
	"context"

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
