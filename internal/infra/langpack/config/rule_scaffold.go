package config

import (
	"context"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// scaffoldRule makes every module wait for the repository scaffold. Dotfiles,
// editor and lint configuration have no reader, so nothing ever pulled them:
// on a 1208-commit history they were born in commits 1142 to 1145, long after
// the code they configure.
//
// The go.mod header is the provider, not the source: every package already
// waits for its module's header, so a scaffold the header waits for is born
// before the first package of the first module without a single edge per
// declaration. The scaffold itself skips the module edge (see moduleRule); a
// file waiting for a go.mod that waits for it would be a cycle.
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
