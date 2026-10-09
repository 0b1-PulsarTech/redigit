package config

import (
	"context"
	"fmt"
	"maps"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// rule is one independently testable edge source, the same shape the Go pack
// uses so both registries read alike.
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

// rules is the compile-time registry. Registering a rule here is what makes it
// run; nothing else discovers a rule implicitly.
var rules = []rule{
	moduleRule{},
	workspaceRule{},
	importRule{},
	checksumRule{},
	scaffoldRule{},
	moduleFilesRule{},
}

// atomRule is the optional second output of a rule: declarations that must be
// born in one commit.
type atomRule interface {
	Atoms(
		ctx context.Context,
		snap entities.Snapshot,
		decls []entities.Decl,
	) (map[entities.DeclKey]string, error)
}

// RulePack contributes the edges that make a build possible and that no
// reference query can see.
type RulePack struct{}

var (
	_ indexing.RulePack = RulePack{}
	_ indexing.Atomizer = RulePack{}
)

func NewRulePack() RulePack { return RulePack{} }

func (RulePack) Name() string { return "config" }

func (RulePack) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	var out []entities.Edge

	for _, each := range rules {
		edges, err := each.Edges(ctx, snap, decls)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", each.Name(), err)
		}

		out = append(out, edges...)
	}

	return out, nil
}

func (RulePack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	var out []entities.Anchor

	for _, each := range rules {
		anchors, err := each.Anchors(ctx, snap, decls)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", each.Name(), err)
		}

		out = append(out, anchors...)
	}

	return out, nil
}

// Atoms unions the groups every rule that makes them names.
func (RulePack) Atoms(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	atoms := map[entities.DeclKey]string{}

	for _, each := range rules {
		grouping, isGrouping := each.(atomRule)
		if !isGrouping {
			continue
		}

		found, err := grouping.Atoms(ctx, snap, decls)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", each.Name(), err)
		}

		maps.Copy(atoms, found)
	}

	return atoms, nil
}
