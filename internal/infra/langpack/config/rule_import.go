package config

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// importRule makes a Go declaration wait for the go.mod "require" providing a
// package its file imports. No reference names the module that ships a package,
// so the first importer landed hundreds of commits before the require it needs
// and every tree between failed with "no required module provides package".
// An import outside the repository is an edge per file, since a require is a leaf
// and the file's imports are written whole; one of the repository's own packages
// is an edge per declaration that reaches it.
type importRule struct{}

func (importRule) Name() string { return "import" }

func (importRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	headers := moduleHeaders(decls)
	if len(headers) == 0 {
		return nil, nil
	}

	roots := slices.Sorted(maps.Keys(headers))
	required := requiredModules(decls)
	byFile := map[string][]entities.Decl{}

	for _, decl := range decls {
		if strings.HasSuffix(decl.Key.File, ".go") {
			byFile[decl.Key.File] = append(byFile[decl.Key.File], decl)
		}
	}

	edges := indirectEdges(snap, decls, headers)
	packages := newPackageIndex(snap, decls, headers)

	for _, file := range slices.Sorted(maps.Keys(byFile)) {
		root, found := nearestModule(roots, file)
		if !found {
			continue
		}

		parsed, fset := parseGoFile(snap, file)
		if parsed == nil {
			continue
		}

		edges = append(edges, fileEdges(parsed, fset, packages, required[root], byFile[file])...)
	}

	return edges, nil
}

func (importRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// fileEdges makes the declarations of one parsed file wait for the requires
// that provide what they import.
func fileEdges(
	parsed *ast.File,
	fset *token.FileSet,
	packages packageIndex,
	required map[string]entities.DeclKey,
	decls []entities.Decl,
) []entities.Edge {
	var edges []entities.Edge

	uses := packages.uses(parsed, fset)
	inRepo := map[string]bool{}

	for _, use := range uses {
		inRepo[use.path] = true
	}

	var outside []string

	for _, imported := range importsOf(parsed) {
		if !inRepo[imported] {
			outside = append(outside, imported)
		}
	}

	for _, provider := range providedRequires(outside, required) {
		for _, dependent := range decls {
			edges = append(edges, entities.Edge{
				Dependent: dependent.Key,
				Provider:  provider,
				Reason:    entities.ReasonImport,
			})
		}
	}

	// A render keeps an import only for the declarations that reach the package,
	// so a require waits for those and not for the whole file: a converter
	// file importing one type of a workspace module otherwise pulled that
	// module's manifest hundreds of commits ahead of its first package.
	for _, use := range uses {
		module, owned := owningModule(use.path, required)
		if !owned {
			continue
		}

		for _, dependent := range decls {
			if len(packages.edges(use, []entities.Decl{dependent})) == 0 {
				continue
			}

			edges = append(edges, entities.Edge{
				Dependent: dependent.Key,
				Provider:  required[module],
				Reason:    entities.ReasonImport,
			})
		}
	}

	for _, use := range uses {
		edges = append(edges, packages.edges(use, decls)...)
	}

	return edges
}

// requiredModules maps each module directory to its require pieces, keyed by
// the module path they require.
func requiredModules(decls []entities.Decl) map[string]map[string]entities.DeclKey {
	out := map[string]map[string]entities.DeclKey{}

	for _, decl := range decls {
		module, isRequire := strings.CutPrefix(decl.Key.Name, requirePrefix)
		if !isRequire || path.Base(decl.Key.File) != goModFile {
			continue
		}

		dir := path.Dir(decl.Key.File)
		if out[dir] == nil {
			out[dir] = map[string]entities.DeclKey{}
		}

		out[dir][module] = decl.Key
	}

	return out
}

// parseGoFile parses file, or yields nil when it cannot be read or parsed: one
// missing edge orders one declaration by history alone, while failing loses
// every edge.
func parseGoFile(snap entities.Snapshot, file string) (*ast.File, *token.FileSet) {
	source, err := readSource(snap, file)
	if err != nil {
		return nil, nil
	}

	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, source, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil
	}

	return parsed, fset
}

// importsOf lists the import paths of a parsed file.
func importsOf(parsed *ast.File) []string {
	out := make([]string, 0, len(parsed.Imports))

	for _, spec := range parsed.Imports {
		if imported, err := strconv.Unquote(spec.Path.Value); err == nil {
			out = append(out, imported)
		}
	}

	return out
}

// providedRequires lists the require pieces of one go.mod that provide a
// package among imports, each once.
func providedRequires(imports []string, required map[string]entities.DeclKey) []entities.DeclKey {
	seen := map[string]bool{}

	var out []entities.DeclKey

	for _, imported := range imports {
		module, found := owningModule(imported, required)
		if !found || seen[module] {
			continue
		}

		seen[module] = true

		out = append(out, required[module])
	}

	return out
}

// owningModule is the longest required module path that is imported, or
// prefixes it at a path boundary. Longest, not first: "example.com/a" and
// "example.com/a/b" can both be required, and the import belongs to the
// deeper one.
func owningModule(imported string, required map[string]entities.DeclKey) (string, bool) {
	best := ""

	for module := range required {
		if imported != module && !strings.HasPrefix(imported, module+"/") {
			continue
		}

		if len(module) > len(best) {
			best = module
		}
	}

	return best, best != ""
}
