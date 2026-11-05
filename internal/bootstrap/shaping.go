package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping/passes"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
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

type revision struct {
	reader   gitstore.Reader
	snapshot entities.Snapshot
	checkout *gitstore.Checkout
}

func openRevision(ctx context.Context, root string, cfg Config) (revision, error) {
	reader := gitstore.NewReader(root, cfg.Rev)

	snapshot, err := reader.Snapshot(ctx)
	if err != nil {
		release(ctx, readerCloser{reader: reader})

		return revision{}, fmt.Errorf("read snapshot: %w", err)
	}

	// Before the checkout and the language server: a rule naming nothing in
	// the tree is a config mistake, and starting gopls costs about 40 s.
	if err = validateRules(cfg, snapshot); err != nil {
		release(ctx, readerCloser{reader: reader})

		return revision{}, err
	}

	// The language server reads a working tree, so the revision has to exist
	// as one. Pointing it at the repository's own checkout would answer about
	// whatever is checked out there instead, and answer confidently.
	checkout, err := gitstore.NewCheckout(ctx, root, cfg.Rev)
	if err != nil {
		release(ctx, readerCloser{reader: reader})

		return revision{}, fmt.Errorf("materialise revision: %w", err)
	}

	// The snapshot describes a REVISION, so its root is where that revision was materialised, not
	// the repository whose working tree holds whatever happens to be checked out. A language pack
	// that reads a sibling file from disk (a generator directive, a mock header) would otherwise
	// read the wrong bytes, or fail on a file the revision has and the checkout does not.
	snapshot.Root = checkout.Dir()

	return revision{reader: reader, snapshot: snapshot, checkout: checkout}, nil
}

func stageHooks() []namedHook {
	return []namedHook{
		{name: "rules", run: ruleHook},
		{name: "shaping", run: shapeHook},
		{name: "lineage", run: lineageHook},
	}
}
