package typecheck

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type Escaped struct{ Source *typed.Source }

func (Escaped) Name() string { return "escaped-methods" }

func (rule Escaped) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	facts, err := rule.Source.Facts(ctx, snap, decls)
	if err != nil {
		return nil, fmt.Errorf("type-check facts: %w", err)
	}

	var edges []entities.Edge

	// Error, Unwrap, Is, As and StatusCode are called at run time even through any/error, so wait for them.
	for _, need := range facts.Escaped {
		edges = append(edges, methodEdges(need, entities.ReasonImplements)...)
	}

	return edges, nil
}

func (Escaped) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
