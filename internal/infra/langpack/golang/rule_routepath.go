package golang

import (
	"context"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/routes"
)

// routePathRule anchors a test that names a URL path to the declarations that
// register the route group serving it. An end-to-end suite talks HTTP, so no
// reference ties it to the handler it drives, and with only the helpers it
// imports to wait for it was born hundreds of commits before the route it
// calls existed. See routes.Groups for what a group is and which one wins.
type routePathRule struct{}

func (routePathRule) Name() string { return "route-path" }

func (routePathRule) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

func (routePathRule) Anchors(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	read := func(file string) ([]byte, error) { return readSource(snap, file) }

	groups := routes.Find(read, decls)
	if len(groups) == 0 {
		return nil, nil
	}

	var anchors []entities.Anchor

	sources := map[string][]byte{}

	for _, decl := range decls {
		if !decl.IsTest && !isTestFile(decl.Key.File) {
			continue
		}

		for _, route := range routes.InGo(routes.Text(read, sources, decl)) {
			for _, group := range groups.Of(route) {
				anchors = append(anchors, entities.Anchor{
					Decl: decl.Key, To: group, Reason: entities.ReasonTestOf,
				})
			}
		}
	}

	return anchors, nil
}

func isTestFile(file string) bool { return strings.HasSuffix(file, "_test.go") }
