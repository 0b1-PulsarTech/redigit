package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
)

// settled runs the two stages every command needs. The ordering is returned
// even when it carries a complaint: a cycle or an unresolved declaration is
// what the caller asked to see, not a reason to print nothing.
func settled(
	ctx context.Context,
	pipeline *bootstrap.Pipeline,
) (entities.Graph, entities.Ordering, error) {
	result, err := pipeline.Graph(ctx)
	if err != nil {
		return entities.Graph{}, entities.Ordering{}, fmt.Errorf("build graph: %w", err)
	}

	warnBlind(result)

	graph := result.Graph

	order, err := pipeline.Order(graph)

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

func withPipeline(
	ctx context.Context,
	cfg bootstrap.Config,
	run func(*bootstrap.Pipeline) error,
) error {
	pipeline, err := bootstrap.Start(ctx, cfg)
	if err != nil {
		return fmt.Errorf("start pipeline: %w", err)
	}

	defer func() { _ = pipeline.Close(context.WithoutCancel(ctx)) }()

	return run(pipeline)
}

func encode(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}

func runAudit(ctx context.Context, cfg bootstrap.Config, out io.Writer) error {
	return withPipeline(ctx, cfg, func(pipeline *bootstrap.Pipeline) error {
		graph, order, err := settled(ctx, pipeline)
		if err != nil {
			return err
		}

		result, err := pipeline.Audit(graph, order)
		if err != nil {
			return fmt.Errorf("audit: %w", err)
		}

		return encode(out, auditDTO{
			Repo:         cfg.Repo,
			Rev:          cfg.Rev,
			BuildTags:    cfg.BuildTags,
			Declarations: graph.Len(),
			Edges:        len(graph.Edges()),
			Rounds:       order.Rounds,
			Integrity:    toMeasures(result.Report.Integrity),
			Shape:        toMeasures(result.Report.Shape),
			Cycles:       toCycles(order.Cycles),
			Drift:        toDrift(result.Drifts),
		})
	})
}

func runGraph(ctx context.Context, cfg bootstrap.Config, out io.Writer) error {
	return withPipeline(ctx, cfg, func(pipeline *bootstrap.Pipeline) error {
		graph, order, err := settled(ctx, pipeline)
		if err != nil {
			return err
		}

		decls := make([]declDTO, 0, graph.Len())
		for _, decl := range graph.Decls() {
			decls = append(decls, declDTO{
				Key:      decl.Key.String(),
				Unit:     string(decl.Unit),
				IsTest:   decl.IsTest,
				Position: int(order.Of(decl.Key)),
			})
		}

		edges := make([]edgeDTO, 0, len(graph.Edges()))
		for _, edge := range graph.Edges() {
			edges = append(edges, edgeDTO{
				Dependent: edge.Dependent.String(),
				Provider:  edge.Provider.String(),
				Reason:    string(edge.Reason),
			})
		}

		return encode(out, graphDTO{Repo: cfg.Repo, Rev: cfg.Rev, Decls: decls, Edges: edges})
	})
}

func runPlan(ctx context.Context, cfg bootstrap.Config, out io.Writer) error {
	return withPipeline(ctx, cfg, func(pipeline *bootstrap.Pipeline) error {
		graph, order, err := settled(ctx, pipeline)
		if err != nil {
			return err
		}

		plan, err := pipeline.Plan(graph, order)
		if err != nil {
			return fmt.Errorf("build plan: %w", err)
		}

		commits := make([]commitDTO, 0, plan.Len())
		for _, commit := range plan.Commits {
			commits = append(commits, commitDTO{
				Position: int(commit.Position),
				Subject:  commit.Subject,
				Shape:    string(commit.Shape),
				Feature:  commit.Feature,
				Reason:   commit.Reason,
				Decls:    toKeys(commit.Decls),
			})
		}

		return encode(out, planDTO{
			Repo:     cfg.Repo,
			Rev:      cfg.Rev,
			Commits:  commits,
			Unplaced: toKeys(plan.Unplaced),
		})
	})
}

// warnBlind names what the resolver could not answer. It goes to the log
// rather than the output stream so a report stays machine-readable, but it is
// never dropped: a graph missing edges looks exactly like code with fewer
// edges, and only this line tells them apart.
func warnBlind(result indexing.Result) {
	if len(result.Blind) == 0 {
		return
	}

	slog.Warn(
		"the resolver could not answer for some declarations",
		slog.Int("count", len(result.Blind)),
		slog.Int("of", result.Graph.Len()),
		slog.String("first", result.Blind[0].Decl.String()),
		slog.String("query", result.Blind[0].Query),
	)
}
