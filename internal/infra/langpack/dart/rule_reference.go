package dart

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// referenceRule makes a declaration wait for every declaration it names, and
// for each export directive the name passes through: a reader importing a
// barrel needs the barrel to offer the name, not only the name to exist.
type referenceRule struct{}

func (referenceRule) Name() string { return "reference" }

func (referenceRule) Edges(
	ctx context.Context,
	space *workspace,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	var out []entities.Edge

	for _, file := range dartFiles(decls) {
		parsed, _, err := space.file(ctx, file)
		if err != nil {
			return nil, err
		}

		visible, err := space.scopeOf(ctx, parsed)
		if err != nil {
			return nil, err
		}

		for _, it := range parsed.decls() {
			for _, ref := range visible.references(it.tokens(parsed.toks), it.key) {
				out = append(out, referenceEdges(it.key, ref.provider)...)
			}
		}
	}

	return out, nil
}

func (referenceRule) Anchors(
	context.Context,
	*workspace,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

func referenceEdges(dependent entities.DeclKey, found provider) []entities.Edge {
	if found.key.IsZero() {
		return nil
	}

	out := make([]entities.Edge, 0, len(found.via)+1)
	out = append(out, entities.Edge{
		Dependent: dependent,
		Provider:  found.key,
		Reason:    entities.ReasonReference,
	})

	for _, export := range found.via {
		out = append(out, entities.Edge{
			Dependent: dependent,
			Provider:  export,
			Reason:    entities.ReasonReference,
		})
	}

	return out
}
