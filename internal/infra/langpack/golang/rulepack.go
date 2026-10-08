package golang

import (
	"context"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
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
	goverterRule{},
	mockgenRule{},
	generateRule{},
	sqlcRule{},
	sqlAtomRule{},
	migrationRule{},
	testAnchorRule{},
	enclosesRule{},
	assertionRule{},
	conversionRule{},
	boundRule{},
	buildTagsRule{},
	embedRule{},
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
	source := newTypedSource(tags)

	return RulePack{rules: append(slices.Clone(rules),
		methodSetRule{source: source},
		typedConversionRule{source: source},
		escapedRule{source: source},
		typedUseRule{source: source},
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
	return collectEdges(ctx, snap, decls, p.rules)
}

func (p RulePack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	return collectAnchors(ctx, snap, decls, p.rules)
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

	for _, r := range p.rules {
		grouping, isGrouping := r.(atomRule)
		if !isGrouping {
			continue
		}

		found, err := grouping.Atoms(ctx, snap, decls)
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

// collectEdges and collectAnchors take the registry as a parameter, rather
// than reading the package var directly, so a test can exercise the
// wrapped-error path with a stub rule instead of mutating global state.
func collectEdges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
	rs []rule,
) ([]entities.Edge, error) {
	var edges []entities.Edge

	for _, r := range rs {
		found, err := r.Edges(ctx, snap, decls)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Name(), err)
		}

		edges = append(edges, found...)
	}

	return edges, nil
}

func collectAnchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
	rs []rule,
) ([]entities.Anchor, error) {
	var anchors []entities.Anchor

	for _, r := range rs {
		found, err := r.Anchors(ctx, snap, decls)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Name(), err)
		}

		anchors = append(anchors, found...)
	}

	return anchors, nil
}
