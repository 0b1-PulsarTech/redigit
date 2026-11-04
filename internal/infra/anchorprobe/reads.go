package anchorprobe

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strings"
)

func (*Probe) Reads(file, text string) ([]string, bool) {
	if !strings.HasSuffix(file, ".go") {
		return nil, false
	}

	parsed, err := parser.ParseFile(token.NewFileSet(), "", "package p\n"+text, 0)
	if err != nil {
		return nil, false
	}

	free := make(map[*ast.Ident]bool, len(parsed.Unresolved))
	names := make(map[string]bool, len(parsed.Unresolved))

	for _, ident := range parsed.Unresolved {
		free[ident] = true
		names[ident.Name] = true
	}

	ast.Inspect(parsed, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.KeyValueExpr:
			// The parser never lists a literal's key: without types it may be a field or a read.
			if key, isIdent := typed.Key.(*ast.Ident); isIdent && key.Obj == nil {
				names[key.Name] = true
			}
		case *ast.SelectorExpr:
			// A free qualifier may be a package, so it keeps its name; any other receiver is a value
			// whose field or method Sel is named, so it is kept as .Sel.
			if qualifier, isIdent := typed.X.(*ast.Ident); isIdent && free[qualifier] {
				names[qualifier.Name+"."+typed.Sel.Name] = true
			} else {
				names["."+typed.Sel.Name] = true
			}
		}

		return true
	})

	return slices.Sorted(maps.Keys(names)), true
}
