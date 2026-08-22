package golang

import (
	"go/ast"
	"strconv"
	"strings"
)

// usedIdents collects every selector prefix ("pkg" in "pkg.Symbol") the kept
// declarations reach. It is a syntactic approximation, not a type-checked
// one: a local identifier that happens to share a package's name would be
// counted as a use.
//
// simplification: no type information is available at this layer, so a
// shadowing local variable can produce a false positive (an import kept that
// nothing actually reaches). It never produces a false negative, which is
// the direction that would break the build.
func usedIdents(decls []ast.Decl) map[string]bool {
	used := map[string]bool{}

	for _, decl := range decls {
		ast.Inspect(decl, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if ident, ok := sel.X.(*ast.Ident); ok {
				used[ident.Name] = true
			}

			return true
		})
	}

	return used
}

// filterImports keeps every import the kept declarations reach by selector,
// plus every blank (_) and dot (.) import: neither can be reached by a
// selector, so both are always kept.
func filterImports(importDecls []ast.Decl, used map[string]bool) []ast.Decl {
	var out []ast.Decl

	for _, decl := range importDecls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		var specs []ast.Spec

		for _, spec := range gen.Specs {
			if is, ok := spec.(*ast.ImportSpec); ok && importIsUsed(is, used) {
				specs = append(specs, spec)
			}
		}

		out = append(out, finishGenDecl(gen, specs)...)
	}

	return out
}

func importIsUsed(spec *ast.ImportSpec, used map[string]bool) bool {
	if spec.Name != nil && (spec.Name.Name == "_" || spec.Name.Name == ".") {
		return true
	}

	return used[importLocalName(spec)]
}

// importLocalName reports the identifier code in this file uses to reach the
// import: its alias, or the last segment of its path.
func importLocalName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}

	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		path = spec.Path.Value
	}

	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}

	return path
}
