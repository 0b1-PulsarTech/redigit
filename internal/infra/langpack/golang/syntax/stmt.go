package syntax

import (
	"go/ast"
	"strconv"
)

const MinGroup = 2

func StmtName(fn string, index int) string {
	return fn + "#" + strconv.Itoa(index)
}

func isListLiteral(lit *ast.CompositeLit) bool {
	switch lit.Type.(type) {
	case *ast.ArrayType, *ast.MapType:
		return true
	}

	// A positional literal of a named type is a struct or a named slice, and no type
	// is known here to tell them apart: dropping an element of a struct breaks it.
	for _, element := range lit.Elts {
		if _, isKeyed := element.(*ast.KeyValueExpr); !isKeyed {
			return false
		}
	}

	return true
}

func ReturnedElements(body *ast.BlockStmt) ([]ast.Expr, func([]ast.Expr)) {
	if len(body.List) != 1 {
		return nil, nil
	}

	ret, isReturn := body.List[0].(*ast.ReturnStmt)
	if !isReturn || len(ret.Results) != 1 {
		return nil, nil
	}

	switch typed := ret.Results[0].(type) {
	case *ast.CompositeLit:
		if !isListLiteral(typed) {
			return nil, nil
		}

		return typed.Elts, func(list []ast.Expr) { typed.Elts = list }
	case *ast.CallExpr:
		selector, isSelector := typed.Fun.(*ast.SelectorExpr)
		if !isSelector || selector.Sel.Name != "Concat" || typed.Ellipsis.IsValid() {
			return nil, nil
		}

		pkg, isIdent := selector.X.(*ast.Ident)
		if !isIdent || pkg.Name != "slices" {
			return nil, nil
		}

		return typed.Args, func(list []ast.Expr) { typed.Args = list }
	}

	return nil, nil
}
