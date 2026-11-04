package golang

import (
	"context"
	"fmt"
	"path"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/buildtags"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/rules/gencode"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/rules/sql"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/rules/tests"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/rules/typecheck"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/rules/wiring"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type (
	rule interface {
		Name() string
		Edges(
			ctx context.Context,
			snap entities.Snapshot,
			decls []entities.Decl,
		) ([]entities.Edge, error)
		Anchors(
			ctx context.Context,
			snap entities.Snapshot,
			decls []entities.Decl,
		) ([]entities.Anchor, error)
	}

	atomRule interface {
		Atoms(
			ctx context.Context,
			snap entities.Snapshot,
			decls []entities.Decl,
		) (map[entities.DeclKey]string, error)
	}
)

var rules = []rule{
	// This slice is the only discovery mechanism; there is no init() registration.
	gencode.Goverter{},
	gencode.Mockgen{},
	gencode.Generate{},
	sql.Sqlc{},
	sql.Atoms{},
	sql.Migration{},
	tests.Anchor{},
	wiring.Encloses{},
	wiring.Assertion{},
	wiring.Conversion{},
	wiring.Bound{},
	buildtags.Rule{},
	wiring.Embed{},
	wiring.MainPackage{},
	tests.RoutePath{},
}

type RulePack struct {
	source *typed.Source
	rules  []rule
}

func NewRulePack(tags []string) RulePack {
	// Loaded lazily once, under the run tags, and shared by the four type-check rules.
	return NewRulePackWith(typed.NewSource(tags))
}

func NewRulePackWith(source *typed.Source) RulePack {
	return RulePack{source: source, rules: append(slices.Clone(rules),
		typecheck.MethodSet{Source: source},
		typecheck.Conversion{Source: source},
		typecheck.Escaped{Source: source},
		typecheck.Reference{Source: source},
	)}
}

var (
	_ indexing.RulePack  = RulePack{}
	_ indexing.Atomizer  = RulePack{}
	_ indexing.Shortfall = RulePack{}
)

func (RulePack) Name() string { return "go" }

func (pack RulePack) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	return collectEdges(ctx, snap, goScope(decls), pack.rules)
}

func (pack RulePack) FilterEdges(snap entities.Snapshot, edges []entities.Edge) []entities.Edge {
	return sql.DropConstruction(snap, edges)
}

func (pack RulePack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	return collectAnchors(ctx, snap, goScope(decls), pack.rules)
}

func goScope(decls []entities.Decl) []entities.Decl {
	out := make([]entities.Decl, 0, len(decls))

	for _, decl := range decls {
		// Drop non-Go declarations: a rule that parses its file fails the whole run.
		if path.Ext(decl.Key.File) == ".go" || decl.Key.Kind == entities.KindOther {
			out = append(out, decl)
		}
	}

	return out
}

func (pack RulePack) Atoms(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	atoms := map[entities.DeclKey]string{}

	scoped := goScope(decls)

	for _, entry := range pack.rules {
		grouping, isGrouping := entry.(atomRule)
		if !isGrouping {
			continue
		}

		found, err := grouping.Atoms(ctx, snap, scoped)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}

		for key, name := range found {
			if _, taken := atoms[key]; !taken {
				atoms[key] = name
			}
		}
	}

	return atoms, nil
}

func (pack RulePack) Partial() []indexing.Partial {
	if pack.source == nil {
		return nil
	}

	var partial []indexing.Partial

	for _, shortfall := range pack.source.Shortfalls() {
		partial = append(partial, indexing.Partial{
			Broken: shortfall.Broken, Of: shortfall.Of, Tags: shortfall.Tags,
			FirstPackage: shortfall.FirstPackage, FirstError: shortfall.FirstError,
			SameMessage: shortfall.SameMessage,
		})
	}

	return partial
}
