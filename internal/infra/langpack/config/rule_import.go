package config

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path"
	"slices"
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

		edges = append(edges, importEdges(parsed, fset, byFile[file], required[root])...)

		for _, use := range packages.uses(parsed, fset) {
			edges = append(edges, packages.edges(use, byFile[file])...)
		}
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

// importEdges makes each declaration of a file wait for the require providing
// a package it uses. The edge was once per file, and a wiring function that
// lists every module of the program then waited for the last require on behalf
// of each of its statements.
func importEdges(
	parsed *ast.File,
	fset *token.FileSet,
	fileDecls []entities.Decl,
	required map[string]entities.DeclKey,
) []entities.Edge {
	uses := importUses(parsed, fset)
	seen := map[entities.Edge]bool{}

	var edges []entities.Edge

	for _, imported := range importsOf(parsed) {
		for _, provider := range providedRequires([]string{imported}, required) {
			for _, dependent := range fileDecls {
				span := declSpan{start: dependent.Span.Start, end: dependent.Span.End}
				if !uses[imported].reaches(span) {
					continue
				}

				edge := entities.Edge{
					Dependent: dependent.Key,
					Provider:  provider,
					Reason:    entities.ReasonImport,
				}
				if !seen[edge] {
					seen[edge] = true

					edges = append(edges, edge)
				}
			}
		}
	}

	return edges
}
