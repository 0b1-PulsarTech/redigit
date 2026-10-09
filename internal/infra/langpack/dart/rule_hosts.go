package dart

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// hostsRule keeps a Dart file behind the pubspec.yaml of its package, without
// which no "package:" import of it resolves, and a workspace member's pubspec
// behind the root one its "resolution: workspace" line reads.
type hostsRule struct{}

func (hostsRule) Name() string { return "hosts" }

func (hostsRule) Edges(
	ctx context.Context,
	space *workspace,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	roots, err := space.packageRoots(ctx)
	if err != nil {
		return nil, err
	}

	dirs := slices.Sorted(maps.Values(roots))
	manifests := manifestDecls(decls)

	var out []entities.Edge

	for _, decl := range decls {
		if path.Ext(decl.Key.File) != dartExt {
			continue
		}

		owner, isOwned := ownerDir(dirs, decl.Key.File)
		if edge, found := manifests.edge(decl.Key, owner, entities.ReasonHosts); isOwned && found {
			out = append(out, edge)
		}
	}

	members, err := workspaceEdges(ctx, space, dirs, manifests)
	if err != nil {
		return nil, err
	}

	return append(out, members...), nil
}

func (hostsRule) Anchors(context.Context, *workspace, []entities.Decl) ([]entities.Anchor, error) {
	return nil, nil
}

func workspaceEdges(
	ctx context.Context,
	space *workspace,
	dirs []string,
	manifests manifestIndex,
) ([]entities.Edge, error) {
	var out []entities.Edge

	for _, dir := range dirs {
		member := path.Join(dir, pubspecName)

		source, err := space.read(ctx, member)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", member, err)
		}

		if pubspecField(string(source), "resolution") != "workspace" {
			continue
		}

		dependent, isDeclared := manifests[dir]
		root, hasRoot := ownerDir(dirs, path.Join(path.Dir(dir), pubspecName))

		if dir == "." || !isDeclared || !hasRoot {
			continue
		}

		if edge, found := manifests.edge(dependent, root, entities.ReasonWorkspace); found {
			out = append(out, edge)
		}
	}

	return out, nil
}

// manifestIndex maps a package directory to the first declaration of its
// pubspec.yaml, whichever pack cut that file.
type manifestIndex map[string]entities.DeclKey

func manifestDecls(decls []entities.Decl) manifestIndex {
	out := manifestIndex{}

	for _, decl := range decls {
		if path.Base(decl.Key.File) != pubspecName {
			continue
		}

		if _, taken := out[path.Dir(decl.Key.File)]; !taken {
			out[path.Dir(decl.Key.File)] = decl.Key
		}
	}

	return out
}

func (m manifestIndex) edge(
	dependent entities.DeclKey,
	dir string,
	reason entities.EdgeReason,
) (entities.Edge, bool) {
	manifest, found := m[dir]

	return entities.Edge{Dependent: dependent, Provider: manifest, Reason: reason}, found
}

// ownerDir is the deepest package directory holding file.
func ownerDir(dirs []string, file string) (string, bool) {
	best, isFound := "", false

	for _, dir := range dirs {
		holds := dir == "." || strings.HasPrefix(file, dir+"/")
		if holds && (!isFound || len(dir) > len(best)) {
			best, isFound = dir, true
		}
	}

	return best, isFound
}
