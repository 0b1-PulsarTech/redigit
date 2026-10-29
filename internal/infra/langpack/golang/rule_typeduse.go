package golang

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// typedUseRule is every reference the type checker resolves, asked of the
// checker instead of the language server. The server answers per declaration
// and refuses some: 1600 of 25758 on the real tree, "no package metadata for
// file" on declarations of files it has no package for. A declaration it
// refuses has no readers, so what reads it lands before it and the trees
// between fail: "undefined: enrichCase" in a test file, "repo.GetByID
// undefined" in another. The checker resolved every identifier of those same
// files already.
type typedUseRule struct{ source *Source }

func (typedUseRule) Name() string { return "typed-reference" }

func (r typedUseRule) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	facts, err := r.source.Facts(ctx, snap, decls)
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

func (typedUseRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
