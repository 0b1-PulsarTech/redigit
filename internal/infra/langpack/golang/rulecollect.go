package golang

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func collectEdges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
	rs []rule,
) ([]entities.Edge, error) {
	var edges []entities.Edge

	for _, r := range rs {
		found, err := r.Edges(ctx, snap, decls)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Name(), err)
		}

		edges = append(edges, found...)
	}

	return edges, nil
}

func collectAnchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
	rs []rule,
) ([]entities.Anchor, error) {
	var anchors []entities.Anchor

	for _, r := range rs {
		found, err := r.Anchors(ctx, snap, decls)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Name(), err)
		}

		anchors = append(anchors, found...)
	}

	return anchors, nil
}
