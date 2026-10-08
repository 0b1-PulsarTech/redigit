package golang

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// usedIdents collects every selector prefix ("pkg" in "pkg.Symbol") the kept
// declarations reach through the file scope. A prefix the parser resolved to a
// local object (a parameter, a range variable, a := binding) is that local,
// not the package: counting it keeps an import nothing uses, and Go rejects
// an unused import. This needs the file parsed WITH object resolution.
//
// simplification: resolution is per file and syntactic, so a package-level
// declaration sharing a package's name is also treated as a local. Go already
// rejects that file, so it cannot occur in a tree that builds.
func usedIdents(decls []ast.Decl) map[string]bool {
	used := map[string]bool{}

	for _, decl := range decls {
		ast.Inspect(decl, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if ident, ok := sel.X.(*ast.Ident); ok && ident.Obj == nil {
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
func filterImports(
	fset *token.FileSet,
	importDecls []ast.Decl,
	used map[string]bool,
) []ast.Decl {
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

		if len(specs) == 0 {
			continue
		}

		closeGroupGaps(fset, gen.Specs, specs)

		gen.Specs = specs
		out = append(out, gen)
	}

	return out
}

// closeGroupGaps moves the surviving specs of each blank-line separated group
// onto the lines the group's first specs held. A dropped sibling then leaves
// no stray blank line, and the groups (standard library, then the rest) stay
// apart: detaching every position instead merges the block, and the printer
// sorts the merged block into one run.
func closeGroupGaps(fset *token.FileSet, original, kept []ast.Spec) {
	for _, spec := range kept {
		if is, ok := spec.(*ast.ImportSpec); !ok || is.Doc != nil || is.Comment != nil {
			return
		}
	}

	keptSet := make(map[ast.Spec]bool, len(kept))
	for _, spec := range kept {
		keptSet[spec] = true
	}

	var (
		group    []ast.Spec
		survivor []ast.Spec
		lastLine int
	)

	flush := func() {
		for index, spec := range survivor {
			place(spec, group[index].Pos())
		}

		group, survivor = nil, nil
	}

	for _, spec := range original {
		line := fset.Position(spec.Pos()).Line
		if len(group) > 0 && line-lastLine > 1 {
			flush()
		}

		group = append(group, spec)

		if keptSet[spec] {
			survivor = append(survivor, spec)
		}

		lastLine = fset.Position(spec.End()).Line
	}

	flush()
}

func place(spec ast.Spec, at token.Pos) {
	is, ok := spec.(*ast.ImportSpec)
	if !ok {
		return
	}

	is.Path.ValuePos = at

	if is.Name != nil {
		is.Name.NamePos = at
		is.Path.ValuePos = at + token.Pos(len(is.Name.Name)+1)
	}

	is.EndPos = token.NoPos
}

func importIsUsed(spec *ast.ImportSpec, used map[string]bool) bool {
	if spec.Name != nil && (spec.Name.Name == "_" || spec.Name.Name == ".") {
		return true
	}

	return used[importLocalName(spec)]
}

// importLocalName reports the identifier code in this file uses to reach the
// import: its alias, or the last segment of its path.
//
// A trailing major-version element is NOT that identifier: "math/rand/v2" is
// reached as rand and "github.com/golang-jwt/jwt/v5" as jwt. Taking the last
// segment blindly yields v2 and v5, matches nothing the code writes, and the
// import is pruned as unused - which produces a file that does not compile,
// found only by rendering a real repository whose dependencies carry versions.
//
// simplification: the local name is still a GUESS from the path. A package
// whose name differs from its final element and carries no alias
// ("gopkg.in/yaml.v3" is handled below, but "example.com/go-thing" declaring
// package thing is not) would still be pruned. Resolving it properly needs the
// imported package's own name, which means loading it; the upgrade path is to
// ask the resolver, which already has that information loaded.
func importLocalName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}

	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		path = spec.Path.Value
	}

	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		last := path[i+1:]
		if isMajorVersion(last) {
			path = path[:i]
			if j := strings.LastIndexByte(path, '/'); j >= 0 {
				path = path[j+1:]
			}
		} else {
			path = last
		}
	}

	// "gopkg.in/yaml.v3" is reached as yaml: the version rides on the element
	// itself rather than on one of its own.
	if dot := strings.LastIndexByte(path, '.'); dot > 0 && isMajorVersion(path[dot+1:]) {
		path = path[:dot]
	}

	return path
}

// isMajorVersion reports the "v2", "v5" form a module path uses for its major
// version. "v" alone is not one, and neither is "v2beta".
func isMajorVersion(part string) bool {
	if len(part) < 2 || part[0] != 'v' {
		return false
	}

	for _, digit := range part[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}

	return true
}
