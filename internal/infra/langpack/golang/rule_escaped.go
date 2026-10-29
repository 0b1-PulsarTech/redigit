package golang

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

// escapedRule makes a declaration that hands a concrete value to an interface
// wait for the methods that interface can reach by name alone. any and error
// list no method the value must have, so the conversion compiles without them,
// yet fmt.Errorf("%w", v), errors.Is, errors.As and a type assertion to a
// duck-typed port call Error, Unwrap, Is, As and StatusCode at run time. A
// tree where the wrapping landed before those methods fails go vet, or behaves
// differently: a mapped 409 became a 500 and errors.Is answered false.
type escapedRule struct{ source *typed.Source }

func (escapedRule) Name() string { return "escaped-methods" }

func (r escapedRule) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	facts, err := r.source.Facts(ctx, snap, decls)
	if err != nil {
		return nil, err
	}

	var edges []entities.Edge

	for _, need := range facts.Escaped {
		edges = append(edges, methodEdges(need, entities.ReasonImplements)...)
	}

	return edges, nil
}

func (escapedRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
