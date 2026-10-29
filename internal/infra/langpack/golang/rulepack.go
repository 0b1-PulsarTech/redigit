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
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/rules/tests"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/rules/wiring"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

// rule is one independently testable edge source. RulePack is a Composite
// over a compile-time slice of these, the shape go/analysis and
// golangci-lint register their analyzers in: no init() side effects, one
// entry appended per rule.
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

// rules is the compile-time registry. Registering a rule here is what makes
// it run; nothing else discovers a rule implicitly.
var rules = []rule{
	gencode.Goverter{},
	gencode.Mockgen{},
	gencode.Generate{},
	Sqlc{},
	Atoms{},
	Migration{},
	tests.Anchor{},
	wiring.Encloses{},
	wiring.Assertion{},
	wiring.Conversion{},
	wiring.Bound{},
	buildtags.Rule{},
	wiring.Embed{},
	tests.RoutePath{},
}

// atomRule is the optional second output of a rule: declarations that must be
// born in one commit, because each needs the other to compile and an edge
// either way would be a cycle.
type atomRule interface {
	Atoms(
		ctx context.Context,
		snap entities.Snapshot,
		decls []entities.Decl,
	) (map[entities.DeclKey]string, error)
}

// RulePack implements indexing.RulePack for Go by fanning out to every
// registered rule and unioning what each finds.
type RulePack struct {
	rules []rule
}

// NewRulePack builds the Go rule pack. The build tags are those the run reads
// the tree under: the rules that ask the type checker check the same files the
// language server answers about. The tree is loaded on the first question and
// shared by every rule that asks.
func NewRulePack(tags []string) RulePack {
	source := typed.NewSource(tags)

	return RulePack{rules: append(slices.Clone(rules),
		MethodSet{Source: source},
		TypedConversion{Source: source},
		Escaped{Source: source},
		Reference{Source: source},
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

// FilterEdges drops the structural edges the sqlc output would otherwise
// carry, whichever side found them.
func (p RulePack) FilterEdges(snap entities.Snapshot, edges []entities.Edge) []entities.Edge {
	return DropConstruction(snap, edges)
}

func (p RulePack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	return collectAnchors(ctx, snap, goScope(decls), p.rules)
}

// goScope keeps what the Go rules may read: Go declarations, and the chunks
// of the files a build needs (go.mod, query files), which are all KindOther.
// A declaration another language pack read is not Go source, and a rule that
// parses its file fails the whole run.
func goScope(decls []entities.Decl) []entities.Decl {
	out := make([]entities.Decl, 0, len(decls))

	for _, decl := range decls {
		if path.Ext(decl.Key.File) == ".go" || decl.Key.Kind == entities.KindOther {
			out = append(out, decl)
		}
	}

	return out
}

// Atoms unions the groups every rule that makes them names. A declaration two
// rules would put in different groups keeps the first, which is a loss of
// co-location and never of a dependency.
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
