package config

import (
	"context"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/routes"
)

// changelogFile is the one document whose pieces are entries about the code, not
// a description of it.
const changelogFile = "CHANGELOG.md"

// changelogRule makes each changelog entry wait for the route groups it names.
// An entry records a change a client can observe, so it is true only once the
// route exists. An entry that names no route is left for the end of the history.
// Each entry that waits also waits for the title, born with the first of them.
type changelogRule struct{}

func (changelogRule) Name() string { return "changelog" }

func (changelogRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	read := func(file string) ([]byte, error) { return readSource(snap, file) }
	groups := routes.Find(read, decls)
	sources := map[string][]byte{}

	var edges []entities.Edge

	headers := map[string]entities.DeclKey{}

	for _, decl := range decls {
		if isChangelogEntry(decl.Key) && decl.Span.Start == 0 &&
			!strings.HasPrefix(routes.Text(read, sources, decl), "## ") &&
			decl.Key.Name == "<file>" {
			headers[decl.Key.File] = decl.Key
		}
	}

	for _, decl := range decls {
		if !isChangelogEntry(decl.Key) || headers[decl.Key.File] == decl.Key {
			continue
		}

		named := map[entities.DeclKey]bool{}

		for _, route := range routes.InProse(routes.Text(read, sources, decl)) {
			for _, group := range groups.Of(route) {
				if named[group] {
					continue
				}

				named[group] = true

				edges = append(edges, entities.Edge{
					Dependent: decl.Key,
					Provider:  group,
					Reason:    entities.ReasonProse,
				})
			}
		}

		if header, found := headers[decl.Key.File]; found && len(named) > 0 {
			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  header,
				Reason:    entities.ReasonProse,
			})
		}
	}

	return edges, nil
}

func (changelogRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

func isChangelogEntry(key entities.DeclKey) bool {
	return isWholeFile(key) && path.Base(key.File) == changelogFile
}
