package di

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

type Rule struct {
	name   string
	cfg    Config
	logger *slog.Logger
	mu     sync.Mutex
	last   finding
}

type finding struct {
	summary                    string
	resolves, untyped, unclear int
}

var (
	_ idiom.EdgeRule = (*Rule)(nil)
	_ idiom.Gater    = (*Rule)(nil)
	_ rules.Globber  = (*Rule)(nil)
)

func Constructor(name string, logger *slog.Logger) rules.Constructor {
	return func(raw json.RawMessage) (rules.Rule, error) {
		cfg, err := decode(raw)
		if err != nil {
			return nil, err
		}

		return &Rule{name: name, cfg: cfg, logger: logger}, nil
	}
}

func (rule *Rule) Type() string { return rule.name }

func (rule *Rule) Globs() []string { return rule.cfg.Paths }

func (rule *Rule) Edges(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
	known idiom.Facts,
) ([]entities.Edge, error) {
	built, err := buildModel(rule.cfg, decls, known)
	if err != nil {
		return nil, err
	}

	rule.report(built)

	return built.edges(), nil
}

//nolint:ireturn // idiom.Gater is declared to hand back the auditor's Gate port.
func (rule *Rule) Gate(factsOf idiom.FactsOf) auditing.Gate {
	return gate{name: rule.name, cfg: rule.cfg, factsOf: factsOf}
}

func (rule *Rule) report(built model) {
	var now finding

	if len(built.registrations) == 0 || built.untyped > 0 || built.ambiguous() > 0 {
		now = finding{
			summary:  built.summary(),
			resolves: len(built.resolves),
			untyped:  built.untyped,
			unclear:  built.ambiguous(),
		}
	}

	rule.mu.Lock()
	defer rule.mu.Unlock()

	// The rule runs once per index pass and a cut can change what it finds: say so when it does.
	if now == rule.last {
		return
	}

	rule.last = now

	if now == (finding{}) {
		rule.logger.Warn(rule.name + ": no finding on the latest pass")

		return
	}

	rule.logger.Warn(rule.name+": "+now.summary,
		slog.Int("resolveSites", now.resolves),
		slog.Int("untypedCalls", now.untyped),
		slog.Int("ambiguousResolves", now.unclear),
	)
}
