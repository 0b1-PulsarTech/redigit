package typecheck

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type Reference struct{ Source *typed.Source }

func (Reference) Name() string { return "typed-reference" }

func (rule Reference) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	facts, err := rule.Source.Facts(ctx, snap, decls)
	if err != nil {
		return nil, err
	}

	edges := make([]entities.Edge, 0, len(facts.Uses))

	for _, named := range facts.Uses {
		edges = append(edges, entities.Edge{
			Dependent: named.Dependent,
			Provider:  named.Provider,
			Reason:    entities.ReasonReference,
		})
	}

	return edges, nil
}

func (Reference) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
