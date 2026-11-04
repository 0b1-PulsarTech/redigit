package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping/passes"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

type (
	shapedPlanner struct {
		inner    planner
		pipeline *shaping.Pipeline
	}

	slogObserver struct{ log *slog.Logger }
)

func shapeHook(_ context.Context, cfg *Config, _ stageEnv, built *stages) error {
	registry, err := shaping.NewRegistry(passes.Entries()...)
	if err != nil {
		return fmt.Errorf("register passes: %w", err)
	}

	list, err := registry.Build(cfg.Sections.Passes)
	if err != nil {
		return fileError(cfg.ConfigPath, err)
	}

	// No passes leaves the planner itself in place: shaping costs nothing then.
	if len(list) > 0 {
		built.planner = shapedPlanner{
			inner:    built.planner,
			pipeline: shaping.NewPipeline(slogObserver{log: slog.Default()}, list...),
		}
	}

	return nil
}

func (shaped shapedPlanner) Plan(
	graph entities.Graph,
	order entities.Ordering,
) (entities.Plan, error) {
	plan, err := shaped.inner.Plan(graph, order)
	if err != nil {
		return plan, fmt.Errorf("plan before shaping: %w", err)
	}

	draft := shaping.NewDraft(graph, plan)

	// The planner port carries no context, so shaping runs to completion.
	if _, err = shaped.pipeline.Apply(context.Background(), draft); err != nil {
		return entities.Plan{}, fmt.Errorf("shape plan: %w", err)
	}

	return draft.Plan(), nil
}

func (observer slogObserver) Moved(pass string, move shaping.Move) {
	if move.Reseat {
		after := slog.Int("after", move.To)
		if move.To == shaping.NoSlot {
			after = slog.String("after", "front")
		}

		observer.log.Debug(
			"shaping reseated after",
			slog.String("pass", pass),
			slog.String("decl", move.Decl.String()),
			slog.Int("slot", move.From),
			after,
		)

		return
	}

	observer.log.Debug(
		"shaping move",
		slog.String("pass", pass),
		slog.String("decl", move.Decl.String()),
		slog.Int("from", move.From),
		slog.Int("to", move.To),
	)
}

func describeBreach(edge entities.Edge) string {
	if edge.Reason == shaping.ReasonAtomSplit {
		return edge.Dependent.String() + " split from its atom with " + edge.Provider.String()
	}

	return edge.Dependent.String() + " needs " + edge.Provider.String()
}

func (observer slogObserver) Refused(pass string, added []entities.Edge) {
	const shown = 5

	edges := make([]string, 0, min(len(added), shown))
	for _, edge := range added[:min(len(added), shown)] {
		edges = append(edges, describeBreach(edge))
	}

	observer.log.Warn(
		"shaping pass refused: it would add hard-edge violations or atom splits",
		slog.String("pass", pass),
		slog.Int("added", len(added)),
		slog.Any("edges", edges),
	)
}

func startInputs(cfg Config) (string, sliceconf.Config, error) {
	root, err := filepath.Abs(cfg.Repo)
	if err != nil {
		return "", sliceconf.Config{}, fmt.Errorf("resolve repository path: %w", err)
	}

	slice, err := decodeSlice(cfg)

	return root, slice, err
}

func stageHooks() []namedHook {
	return []namedHook{
		{name: "rules", run: ruleHook},
		{name: "shaping", run: shapeHook},
		{name: "lineage", run: lineageHook},
	}
}
