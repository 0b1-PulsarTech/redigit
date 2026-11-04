package router

import (
	"context"
	"encoding/json"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

type Rule struct {
	name string
	cfg  Config
}

var (
	_ idiom.AtomRule = (*Rule)(nil)
	_ idiom.Gater    = (*Rule)(nil)
	_ rules.Globber  = (*Rule)(nil)
)

func Constructor(name string) rules.Constructor {
	return func(raw json.RawMessage) (rules.Rule, error) {
		cfg, err := decode(raw)
		if err != nil {
			return nil, err
		}

		return &Rule{name: name, cfg: cfg}, nil
	}
}

func (rule *Rule) Type() string { return rule.name }

func (rule *Rule) Globs() []string { return rule.cfg.Paths }

func (rule *Rule) Atoms(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
	known idiom.Facts,
) (map[entities.DeclKey]string, error) {
	view, err := build(rule.cfg, decls, known)
	if err != nil {
		return nil, err
	}

	return view.withoutSharedCallers(rule.cfg, decls).atoms(rule.cfg), nil
}

func (rule *Rule) Routes(decls []entities.Decl, known idiom.Facts) (View, error) {
	return build(rule.cfg, decls, known)
}

//nolint:ireturn // idiom.Gater is declared to hand back the auditor's Gate port.
func (rule *Rule) Gate(factsOf idiom.FactsOf) auditing.Gate {
	return gate{name: rule.name, cfg: rule.cfg, factsOf: factsOf}
}
