package typecheck

import (
	"context"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type MethodSet struct{ Source *typed.Source }

func (MethodSet) Name() string { return "method-set" }

func (r MethodSet) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	facts, err := r.Source.Facts(ctx, snap, decls)
	if err != nil {
		return nil, err
	}

	var edges []entities.Edge

	for _, need := range facts.Forced {
		if need.Dependent == need.Owner {
			continue
		}

		edges = append(edges, methodEdges(need, entities.ReasonMethodSet)...)
	}

	return edges, nil
}

func (MethodSet) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

func (r MethodSet) Atoms(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	facts, err := r.Source.Facts(ctx, snap, decls)
	if err != nil {
		return nil, err
	}

	atoms := map[entities.DeclKey]string{}

	for _, need := range facts.Forced {
		if need.Dependent != need.Owner {
			continue
		}

		name := "method-set:" + need.Owner.String()
		atoms[need.Owner] = name

		for _, method := range need.Methods {
			atoms[method] = name
		}
	}

	return atoms, nil
}

func methodEdges(need typed.Requirement, reason entities.EdgeReason) []entities.Edge {
	edges := make([]entities.Edge, 0, len(need.Methods))

	for _, method := range need.Methods {
		if method == need.Dependent {
			continue
		}

		edges = append(edges, entities.Edge{
			Dependent: need.Dependent,
			Provider:  method,
			Reason:    reason,
		})
	}

	return slices.Clip(edges)
}
