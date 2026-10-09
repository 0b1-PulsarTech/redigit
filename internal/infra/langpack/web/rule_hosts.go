package web

import (
	"context"
	"path"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// hostsRule keeps a file behind the package.json and tsconfig.json nearest to
// it: they are where its "#" imports, its package name and its type paths
// resolve, so without them nothing it imports resolves.
type hostsRule struct{}

func (hostsRule) Name() string { return "hosts" }

func (hostsRule) Edges(_ context.Context, run *index) ([]entities.Edge, error) {
	manifests := map[string]map[string]entities.DeclKey{packageJSON: {}, tsconfigJSON: {}}

	for file := range run.byFile {
		if byDir, isManifest := manifests[path.Base(file)]; isManifest {
			byDir[path.Dir(file)], _ = run.first(file)
		}
	}

	var out []entities.Edge

	for _, file := range run.scripts {
		dependent, _ := run.first(file)

		for _, byDir := range manifests {
			if _, provider, found := nearest(byDir, path.Dir(file)); found {
				out = append(out, entities.Edge{
					Dependent: dependent,
					Provider:  provider,
					Reason:    entities.ReasonHosts,
				})
			}
		}
	}

	return out, nil
}

func (hostsRule) Anchors(context.Context, *index) ([]entities.Anchor, error) { return nil, nil }
