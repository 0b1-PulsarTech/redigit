package typecheck

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

// Conversion makes a declaration that hands a concrete value to an
// interface wait for the methods of that value the interface lists. A reference
// query sees the value's TYPE at most, and the line compiles only once every
// method the interface demands exists: "cannot use budgettemplaterepo.New(db)
// (value of struct type Repo) as ...Budget" failed the trees where the wiring
// landed before the repository's methods. The pairing of value and interface is
// the type checker's, so it covers every place Go converts silently (argument,
// assignment, return, literal element, send, type assertion) instead of the few
// a text rule could name.
type Conversion struct{ Source *typed.Source }

func (Conversion) Name() string { return "typed-conversion" }

func (r Conversion) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	facts, err := r.Source.Facts(ctx, snap, decls)
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
