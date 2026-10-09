package config

import (
	"context"
	"path"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/routes"
)

// changelogFile is the one document whose pieces are entries about the code, not
// a description of it.
const changelogFile = "CHANGELOG.md"

// changelogRule makes each changelog entry wait for the route groups it names.
// An entry records a change a client can observe, so it is true only once the
// route exists: the file cut by lines and born whole came before any of the code
// its entries are about. An entry that names no route has nothing to wait for and
// is left for the end of the history.
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

	for _, decl := range decls {
		if !isChangelogEntry(decl.Key) {
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
