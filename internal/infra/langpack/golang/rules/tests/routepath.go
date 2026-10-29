package tests

import (
	"context"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/routes"
)

type RoutePath struct{}

func (RoutePath) Name() string { return "route-path" }

func (RoutePath) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

func (RoutePath) Anchors(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	read := func(file string) ([]byte, error) { return declsource.ReadSource(snap, file) }

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
