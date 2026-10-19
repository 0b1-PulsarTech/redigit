package config

import (
	"context"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// workspaceRule ties a line naming a module DIRECTORY - a go.work "use ./x",
// a go.mod "replace y => ../y" - to the go.mod that makes it exist. Go reads a
// replaced directory even when nothing imports the module, so a replace
// landing before its target breaks EVERY build until it arrives: that broke
// 53 trees on a real repository.
type workspaceRule struct{}

func (workspaceRule) Name() string { return "workspace" }

func (workspaceRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	headers := moduleHeaders(decls)
	if len(headers) == 0 {
		return nil, nil
	}

	sources := map[string][]byte{}
	replaced := map[string]string{}

	works := workHeaders(decls)

	var edges []entities.Edge

	for _, decl := range decls {
		target, names := localTarget(decl, snap, sources)
		if !names {
			continue
		}

		provider, found := headers[target]
		if !found {
			continue
		}

		if module, isReplace := strings.CutPrefix(decl.Key.Name, replacePrefix); isReplace {
			replaced[decl.Key.File+"\x00"+module] = target
		}

		edges = append(edges, entities.Edge{
			Dependent: decl.Key,
			Provider:  provider,
			Reason:    entities.ReasonWorkspace,
		})

		if work, hasWork := works[decl.Key.File]; hasWork && isUse(decl.Key) {
			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  work,
				Reason:    entities.ReasonWorkspace,
			})
		}
	}

	edges = append(edges, requireEdges(decls, headers, replaced)...)

	return append(edges, directiveEdges(decls, headers)...), nil
}

// directiveEdges makes a require or replace line wait for the header of its own
// go.mod. A line born before the module it belongs to leaves a manifest with a
// directive and no module, and starts the module's history at the line instead
// of at its first package.
func directiveEdges(
	decls []entities.Decl,
	headers map[string]entities.DeclKey,
) []entities.Edge {
	var edges []entities.Edge

	for _, decl := range decls {
		if path.Base(decl.Key.File) != goModFile ||
			!strings.HasPrefix(decl.Key.Name, requirePrefix) &&
				!strings.HasPrefix(decl.Key.Name, replacePrefix) {
			continue
		}

		if header, found := headers[path.Dir(decl.Key.File)]; found {
			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  header,
				Reason:    entities.ReasonModule,
			})
		}
	}

	return edges
}

// localTarget reports the repository-relative directory a piece names. A
// replace whose right-hand side is a version rather than a path names none:
// "replace X v1.0.0 => X v1.1.0" needs nothing to exist in the tree.
func localTarget(
	decl entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) (string, bool) {
	base := path.Dir(decl.Key.File)

	switch {
	case strings.HasPrefix(decl.Key.Name, usePrefix) &&
		path.Base(decl.Key.File) == goWorkFile:
		return resolve(base, strings.TrimSpace(strings.TrimPrefix(decl.Key.Name, usePrefix)))

	case strings.HasPrefix(decl.Key.Name, replacePrefix) &&
		path.Base(decl.Key.File) == goModFile:
		return replaceTarget(base, decl, snap, sources)
	}

	return "", false
}

// replaceTarget reads the right-hand side out of the replace line's own bytes:
// the NAME carries only the module replaced, the target lives in the span's
// text. An unreadable file yields no edge rather than an error, because one
// missing edge orders one line by history while failing loses every edge.
func replaceTarget(
	base string,
	decl entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) (string, bool) {
	source, cached := sources[decl.Key.File]
	if !cached {
		read, err := readSource(snap, decl.Key.File)
		if err != nil {
			read = nil
		}

		source = read
		sources[decl.Key.File] = source
	}

	_, arrow, found := strings.Cut(sliceSpan(source, decl.Span), "=>")
	if !found {
		return "", false
	}

	fields := strings.Fields(arrow)
	if len(fields) == 0 {
		return "", false
	}

	return resolve(base, fields[0])
}

// resolve turns a path written relative to the file that holds it into a
// repository-relative one. A target that is not a local path - a module path
// with no dot-slash - names no directory in this tree.
func resolve(base, target string) (string, bool) {
	if !strings.HasPrefix(target, "./") && !strings.HasPrefix(target, "../") {
		return "", false
	}

	return path.Clean(path.Join(base, target)), true
}
