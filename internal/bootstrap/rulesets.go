package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/gofacts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/di"
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
		"go-router": router.Constructor("go-router"),
		"pin":       pinrule.Constructor("pin"),
	}
}
