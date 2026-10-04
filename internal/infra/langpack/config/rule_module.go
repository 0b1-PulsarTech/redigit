package config

import (
	"context"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// The names the go.mod and go.work chunkers give their pieces. A go.mod's
// header piece is named by the module path itself; every other piece carries
// the directive it was found under.
const (
	requirePrefix = "require "
	replacePrefix = "replace "
	usePrefix     = "use "
	goModFile     = "go.mod"
	goWorkFile    = "go.work"
	goSumFile     = "go.sum"
)

// moduleRule is the edge no reference query can see: a package compiles only
// where a go.mod makes its directory a module, and a go.mod names none of the
// packages it covers. Without it, every tree before the go.mod lands has
// source and no module to build it, which made `go build` unverifiable.
type moduleRule struct{}

func (moduleRule) Name() string { return "module" }

func (moduleRule) Edges(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	headers := moduleHeaders(decls)
	if len(headers) == 0 {
		return nil, nil
	}

	roots := slices.Sorted(maps.Keys(headers))

	var edges []entities.Edge

	for _, decl := range decls {
		root, found := nearestModule(roots, decl.Key.File)
		if !found || decl.Key.File == path.Join(root, goModFile) {
			continue
		}

		edges = append(edges, entities.Edge{
			Dependent: decl.Key,
			Provider:  headers[root],
			Reason:    entities.ReasonModule,
		})
	}

	return edges, nil
}

func (moduleRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// moduleHeaders maps each module's directory to its go.mod header piece, the
// one that carries the "module" directive.
func moduleHeaders(decls []entities.Decl) map[string]entities.DeclKey {
	out := map[string]entities.DeclKey{}

	for _, decl := range decls {
		if path.Base(decl.Key.File) != goModFile {
			continue
		}

		// The header is the only piece not named after a directive.
		if strings.HasPrefix(decl.Key.Name, requirePrefix) ||
			strings.HasPrefix(decl.Key.Name, replacePrefix) {
			continue
		}

		out[path.Dir(decl.Key.File)] = decl.Key
	}

	return out
}

// nearestModule reports the deepest module directory containing file. Deepest,
// not first: a workspace nests modules inside each other's directories, and
// the outer one does not compile the inner one's packages.
func nearestModule(roots []string, file string) (string, bool) {
	best := ""
	found := false

	for _, root := range roots {
		if !within(root, file) {
			continue
		}

		if !found || len(root) > len(best) {
			best, found = root, true
		}
	}

	return best, found
}

func within(root, file string) bool {
	if root == "." {
		return true
	}

	return strings.HasPrefix(file, root+"/")
}
