package config

import (
	"context"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// scaffoldRule makes every module wait for the repository scaffold: dotfiles and
// lint configuration have no reader. The go.mod header is the provider, so the
// scaffold is born before the first package without an edge per declaration.
type scaffoldRule struct{}

func (scaffoldRule) Name() string { return "scaffold" }

func (scaffoldRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	var edges []entities.Edge

	forEachScaffoldHeader(snap, decls, func(file, header entities.DeclKey) {
		edges = append(edges, entities.Edge{
			Dependent: header,
			Provider:  file,
			Reason:    entities.ReasonHosts,
		})
	})

	return edges, nil
}

// Anchors lower the scaffold to the earliest header. A dotfile is first
// touched wherever the author happened to add it, and the fixed point only
// raises a header to the position of what it waits for.
func (scaffoldRule) Anchors(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	var anchors []entities.Anchor

	forEachScaffoldHeader(snap, decls, func(file, header entities.DeclKey) {
		anchors = append(anchors, entities.Anchor{
			Decl:   file,
			To:     header,
			Reason: entities.ReasonHosts,
		})
	})

	return anchors, nil
}

func forEachScaffoldHeader(
	snap entities.Snapshot,
	decls []entities.Decl,
	visit func(file, header entities.DeclKey),
) {
	headers := moduleHeaders(decls)
	roots := slices.Sorted(maps.Keys(headers))
	placed := classify(snap, decls)

	for _, decl := range decls {
		if placed[decl.Key].role != roleScaffold {
			continue
		}

		for _, root := range roots {
			visit(decl.Key, headers[root])
		}
	}
}
