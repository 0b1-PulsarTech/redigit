package cli

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

//go:generate go tool -modfile=../../../tools/go.mod mockgen -source=pipeline.go -destination=pipeline_mock_test.go -package=cli

// pipeline is what a command needs from a running pipeline: enough to build
// the graph, settle an ordering, plan or audit it, and shut down cleanly.
// Declaring it here, in the consumer, lets a test supply a fake instead of
// bootstrap.Start's language server and git worktree - the two things that
// make a *bootstrap.Pipeline impossible to drive as a unit.
type pipeline interface {
	Snapshot() entities.Snapshot
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

var _ pipeline = (*bootstrap.Pipeline)(nil)

// starter builds a pipeline for one run. Run wires it to bootstrap.Start; a
// test wires it to a fake pipeline instead.
type starter func(ctx context.Context, cfg bootstrap.Config) (pipeline, error)

// realStart adapts bootstrap.Start to the starter seam: the one place a
// concrete *bootstrap.Pipeline crosses into the pipeline port every command
// depends on.
//
//nolint:ireturn // see above; the interface return is the point of this func.
func realStart(ctx context.Context, cfg bootstrap.Config) (pipeline, error) {
	pl, err := bootstrap.Start(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("bootstrap start: %w", err)
	}

	return pl, nil
}
