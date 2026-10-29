package render

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

func usedIdents(decls []ast.Decl) map[string]bool {
	used := map[string]bool{}

	for _, decl := range decls {
		ast.Inspect(decl, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// A resolved Obj is a local binding, not a package; counting it keeps an import Go rejects as unused.
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Obj == nil {
				used[ident.Name] = true
			}

			return true
		})
	}

	return used
}

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

func closeGroupGaps(fset *token.FileSet, original, kept []ast.Spec) {
	for _, spec := range kept {
		if is, ok := spec.(*ast.ImportSpec); !ok || is.Doc != nil || is.Comment != nil {
			return
		}
	}

	// Regroup per blank-line block: detaching every position merges the blocks and the printer sorts them.
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

func importLocalName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}

	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		path = spec.Path.Value
	}

	// The name is a guess from the path; skip /vN (math/rand/v2 is rand) or the import is
	// pruned and the output does not compile.
	if slash := strings.LastIndexByte(path, '/'); slash >= 0 {
		last := path[slash+1:]
		if isMajorVersion(last) {
			path = path[:slash]
			if inner := strings.LastIndexByte(path, '/'); inner >= 0 {
				path = path[inner+1:]
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
