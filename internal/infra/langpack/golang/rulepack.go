package golang

import (
	"context"
	"fmt"

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
	sqlcRule{},
	migrationRule{},
	testAnchorRule{},
	enclosesRule{},
	assertionRule{},
	buildTagsRule{},
}

// RulePack implements indexing.RulePack for Go by fanning out to every
// registered rule and unioning what each finds.
type RulePack struct{}

// NewRulePack builds the Go rule pack. It carries no state: every call reads
// only the snapshot and declarations it is given.
func NewRulePack() RulePack { return RulePack{} }

var _ indexing.RulePack = RulePack{}

func (RulePack) Name() string { return "go" }

func (RulePack) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	return collectEdges(ctx, snap, decls, rules)
}

func (RulePack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	return collectAnchors(ctx, snap, decls, rules)
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
