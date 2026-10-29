package golang

import (
	"go/ast"
	"strconv"
)

// MinGroup is the fewest pieces that make an atom.
const MinGroup = 2

// StmtName is the name of the index-th statement of the function named fn.
func StmtName(fn string, index int) string {
	return fn + "#" + strconv.Itoa(index)
}

// ReturnedElements are the pieces of a body that is one return of a list: the
// elements of a composite literal, or the arguments of slices.Concat. Each can
// be left out and the rest still compiles, and a list that grows one element
// per feature is a registration table. The second result writes a new list.
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
