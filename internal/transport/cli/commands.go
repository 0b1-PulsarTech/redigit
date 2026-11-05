package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/transport/cli/session"
)

func runAudit(ctx context.Context, cfg bootstrap.Config, out io.Writer, start starter) error {
	return session.WithPipeline(ctx, cfg, start, func(pl pipeline) error { //nolint:wrapcheck
		graph, order, err := settled(ctx, pl)
		if err != nil {
			return err
		}

		result, err := pl.Audit(graph, order)
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
			Unseen:       toUnseen(result.Unseen),
			Unclaimed:    toUnclaimed(result.Unclaimed),
			Integrity:    toMeasures(result.Report.Integrity),
			Shape:        toMeasures(result.Report.Shape),
			Cycles:       toCycles(order.Cycles),
			Drift:        toDrift(result.Drifts),
		})
	})
}

func runGraph(ctx context.Context, cfg bootstrap.Config, out io.Writer, start starter) error {
	return session.WithPipeline(ctx, cfg, start, func(pl pipeline) error { //nolint:wrapcheck
		graph, order, err := settled(ctx, pl)
		if err != nil {
			return err
		}

		decls := make([]declDTO, 0, graph.Len())
		for _, decl := range graph.Decls() {
			decls = append(decls, declDTO{
				Key:      decl.Key.String(),
				Unit:     string(decl.Unit),
				IsTest:   decl.IsTest,
				Atom:     decl.Atom,
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

func runPlan(ctx context.Context, cfg bootstrap.Config, out io.Writer, start starter) error {
	return session.WithPipeline(ctx, cfg, start, func(pl pipeline) error { //nolint:wrapcheck
		graph, order, err := settled(ctx, pl)
		if err != nil {
			return err
		}

		plan, err := pl.Plan(graph, order)
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
				Changes:  toKeys(commit.Changes),
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
