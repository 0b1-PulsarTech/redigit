package golang

import (
	"context"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// methodSetRule keeps the methods a type's own declaration requires of it
// with the type. "type ErrNotFound struct{ BaseError[ErrNotFound,
// *ErrNotFound] }" instantiates a generic whose constraint demands Error() of
// the pointer, so the declaration compiles only once that method exists - and a
// method waits for its receiver, so no edge can say it. Making the type wait for
// the method closes a cycle. The two are one unit instead: the methods the
// constraint names join the type's atom, and no others do. That failed 72 of the
// 98 red sampled trees with "does not satisfy ... (missing method Error)".
//
// When the instantiation is in some OTHER declaration, the methods are plain
// edges: that declaration reads them, the type does not need them to exist.
type methodSetRule struct{ source *typedSource }

func (methodSetRule) Name() string { return "method-set" }

func (r methodSetRule) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	facts, err := r.source.Facts(ctx, snap, decls)
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

func (methodSetRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// Atoms groups each type that requires methods of itself with those methods.
func (r methodSetRule) Atoms(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	facts, err := r.source.Facts(ctx, snap, decls)
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

// methodEdges turns a requirement into one edge per method, dropping a method
// that is the dependent itself: a declaration does not wait for itself.
func methodEdges(need requirement, reason entities.EdgeReason) []entities.Edge {
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
