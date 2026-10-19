package config

import (
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// requireSets are the require pieces of every go.mod: the direct ones, the
// "// indirect" ones, and every one by the module it requires.
type requireSets struct {
	direct   map[string][]entities.DeclKey
	indirect map[string][]entities.DeclKey
	byModule map[string]map[string]entities.DeclKey
}

func newRequireSets(snap entities.Snapshot, decls []entities.Decl) requireSets {
	sets := requireSets{
		direct:   map[string][]entities.DeclKey{},
		indirect: map[string][]entities.DeclKey{},
		byModule: requiredModules(decls),
	}
	sources := map[string][]byte{}

	for _, decl := range decls {
		if !strings.HasPrefix(decl.Key.Name, requirePrefix) ||
			path.Base(decl.Key.File) != goModFile {
			continue
		}

		dir := path.Dir(decl.Key.File)
		if isIndirect(snap, decl, sources) {
			sets.indirect[dir] = append(sets.indirect[dir], decl.Key)

			continue
		}

		sets.direct[dir] = append(sets.direct[dir], decl.Key)
	}

	return sets
}

// indirectEdges makes every direct require wait for the "// indirect" requires
// of its own go.mod. A module graph is pruned: the go command wants every
// module providing a package the build transitively imports listed in the main
// go.mod, and answers "updates to go.mod needed" until it is. Which direct
// require needs which indirect one is the dependency's own go.mod, which a
// tree under construction does not hold, so each direct require pulls them all.
//
// A module of the workspace is the exception: its own go.mod is in the
// repository, so an indirect require of it is born once a workspace module this
// go.mod requires lists it, and no direct require waits for it. Waiting for all
// of them made the first consumer of any library carry the manifest of every
// module, hundreds of commits before that module's first package.
func indirectEdges(
	snap entities.Snapshot,
	decls []entities.Decl,
	headers map[string]entities.DeclKey,
) []entities.Edge {
	sets := newRequireSets(snap, decls)
	roots := map[string]string{}

	for root, header := range headers {
		roots[header.Name] = root
	}

	var edges []entities.Edge

	for _, dir := range slices.Sorted(maps.Keys(sets.direct)) {
		for _, provider := range sets.indirect[dir] {
			module := strings.TrimPrefix(provider.Name, requirePrefix)
			if _, local := roots[module]; local {
				edges = append(edges, sets.listedBy(dir, module, provider, roots)...)

				continue
			}

			for _, dependent := range sets.direct[dir] {
				edges = append(edges, entities.Edge{
					Dependent: dependent,
					Provider:  provider,
					Reason:    entities.ReasonImport,
				})
			}
		}
	}

	return edges
}

// listedBy makes an indirect require of a workspace module wait for the require
// of the same module in the go.mod of each workspace module that dir requires.
func (sets requireSets) listedBy(
	dir, module string,
	indirect entities.DeclKey,
	roots map[string]string,
) []entities.Edge {
	var edges []entities.Edge

	for _, direct := range sets.direct[dir] {
		root, local := roots[strings.TrimPrefix(direct.Name, requirePrefix)]
		if !local {
			continue
		}

		if listing, listed := sets.byModule[root][module]; listed {
			edges = append(edges, entities.Edge{
				Dependent: indirect,
				Provider:  listing,
				Reason:    entities.ReasonImport,
			})
		}
	}

	return edges
}

// isIndirect reports whether a require piece carries the "// indirect" comment.
// An unreadable go.mod makes it direct: that adds no edge, so the cost of the
// guess is one line ordered by history.
func isIndirect(snap entities.Snapshot, decl entities.Decl, sources map[string][]byte) bool {
	source, cached := sources[decl.Key.File]
	if !cached {
		source, _ = readSource(snap, decl.Key.File)
		sources[decl.Key.File] = source
	}

	_, comment, found := strings.Cut(sliceSpan(source, decl.Span), "//")

	return found && strings.HasPrefix(strings.TrimSpace(comment), "indirect")
}
