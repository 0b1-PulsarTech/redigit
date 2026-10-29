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

type rule interface {
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

var rules = []rule{
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
	tests.RoutePath{},
}

type atomRule interface {
	Atoms(
		ctx context.Context,
		snap entities.Snapshot,
		decls []entities.Decl,
	) (map[entities.DeclKey]string, error)
}

type RulePack struct {
	rules []rule
}

func NewRulePack(tags []string) RulePack {
	source := typed.NewSource(tags)

	return RulePack{rules: append(slices.Clone(rules),
		typecheck.MethodSet{Source: source},
		typecheck.Conversion{Source: source},
		typecheck.Escaped{Source: source},
		typecheck.Reference{Source: source},
	)}
}

var (
	_ indexing.RulePack = RulePack{}
	_ indexing.Atomizer = RulePack{}
)

func (RulePack) Name() string { return "go" }

func (p RulePack) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	return collectEdges(ctx, snap, goScope(decls), p.rules)
}

func (p RulePack) FilterEdges(snap entities.Snapshot, edges []entities.Edge) []entities.Edge {
	return sql.DropConstruction(snap, edges)
}

func (p RulePack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	return collectAnchors(ctx, snap, goScope(decls), p.rules)
}

func goScope(decls []entities.Decl) []entities.Decl {
	out := make([]entities.Decl, 0, len(decls))

	for _, decl := range decls {
		if path.Ext(decl.Key.File) == ".go" || decl.Key.Kind == entities.KindOther {
			out = append(out, decl)
		}
	}

	return out
}

func (p RulePack) Atoms(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	atoms := map[entities.DeclKey]string{}

	scoped := goScope(decls)

	for _, r := range p.rules {
		grouping, isGrouping := r.(atomRule)
		if !isGrouping {
			continue
		}

		found, err := grouping.Atoms(ctx, snap, scoped)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Name(), err)
		}

		for key, name := range found {
			if _, taken := atoms[key]; !taken {
				atoms[key] = name
			}
		}
	}

	return atoms, nil
}
