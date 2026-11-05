package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/gofacts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/di"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/e2e"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/router"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/pinrule"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

type goFacts struct{ builder *gofacts.Builder }

//nolint:ireturn // the idiom port's own Facts is what every rule consumes.
func (provider goFacts) Facts(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (idiom.Facts, error) {
	index, err := provider.builder.Index(ctx, snap, decls)
	if err != nil {
		return nil, fmt.Errorf("go facts: %w", err)
	}

	return index, nil
}

func ruleHook(ctx context.Context, cfg *Config, env stageEnv, built *stages) error {
	var (
		idioms []rules.Rule
		pins   []*pinrule.Rule
	)

	for _, rule := range cfg.Rules {
		switch typed := rule.(type) {
		case idiom.EdgeRule, idiom.AnchorRule, idiom.AtomRule, idiom.Gater:
			idioms = append(idioms, rule)
		case *pinrule.Rule:
			pins = append(pins, typed)
		}
	}

	if len(idioms) > 0 {
		pack := idiom.NewPack(goFacts{gofacts.New(env.goSource)}, []string{".go"}, idioms...).
			WithLogger(slog.Default())
		built.languages = append(built.languages, indexing.Language{Name: "go-idioms", Rules: pack})
		cfg.Auditing.Gates = append(cfg.Auditing.Gates, pack.Gates(ctx, env.opened.snapshot)...)
	}

	// Pins read decls alone, so they ride a language of their own and never force the typed load.
	if len(pins) > 0 {
		built.languages = append(
			built.languages,
			indexing.Language{Name: "pins", Rules: pinrule.NewPack(pins...)},
		)
	}

	return nil
}

func ruleConstructors() map[string]rules.Constructor {
	// Each idiom pack registers its own constructor here.
	return map[string]rules.Constructor{
		"go-di":     di.Constructor("go-di", slog.Default()),
		"go-e2e":    e2e.Constructor("go-e2e", slog.Default()),
		"go-router": router.Constructor("go-router"),
		"pin":       pinrule.Constructor("pin"),
	}
}

type (
	stageHook func(context.Context, *Config, stageEnv, *stages) error

	namedHook struct {
		name string
		run  stageHook
	}

	stageEnv struct {
		root     string
		opened   revision
		specific []indexing.Language
		goSource *typed.Source
		tree     finalTree
	}

	stages struct {
		languages []indexing.Language
		orderer   orderer
		planner   planner
		closers   closers
	}
)

func shapeStages(
	ctx context.Context,
	cfg *Config,
	env stageEnv,
	configSyntax config.Syntax,
	hooks []namedHook,
) (indexing.Config, *stages, error) {
	if err := validateChunkMap(cfg.ChunkMap, env.opened.snapshot, env.specific); err != nil {
		return indexing.Config{}, nil, fmt.Errorf("validate chunk map: %w", err)
	}

	built := &stages{orderer: ordering.New(cfg.Ordering), planner: planning.New(cfg.Planning)}

	if err := runHooks(ctx, cfg, env, built, hooks); err != nil {
		return indexing.Config{}, nil, err
	}

	indexCfg := cfg.Indexing
	indexCfg.Languages = slices.Concat(
		env.specific, built.languages, []indexing.Language{opaqueLanguage(configSyntax)},
	)

	return indexCfg, built, nil
}

func runHooks(
	ctx context.Context,
	cfg *Config,
	env stageEnv,
	built *stages,
	hooks []namedHook,
) error {
	for _, hook := range hooks {
		if err := hook.run(ctx, cfg, env, built); err != nil {
			release(ctx, built.closers)

			return fmt.Errorf("%s: %w", hook.name, err)
		}
	}

	return nil
}
