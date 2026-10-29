package typecheck

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type Conversion struct{ Source *typed.Source }

func (Conversion) Name() string { return "typed-conversion" }

func (rule Conversion) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	facts, err := rule.Source.Facts(ctx, snap, decls)
	if err != nil {
		return nil, err
	}

	var edges []entities.Edge

	for _, need := range facts.Converted {
		edges = append(edges, methodEdges(need, entities.ReasonImplements)...)
	}

	return edges, nil
}

func (Conversion) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
