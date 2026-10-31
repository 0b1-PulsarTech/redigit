package typed

import (
	"go/ast"
	"go/token"
	"go/types"
)

func (visitor *conversionVisitor) convertEach(
	rhs []ast.Expr,
	count int,
	target func(index int) types.Type,
) {
	// A lone tuple-valued call (or comma-ok form) spreads over every target.
	if len(rhs) == 1 {
		visitor.convertTuple(rhs[0], count, target)
	}

	if len(rhs) != count {
		return
	}

	checked := visitor.scan.pkg.TypesInfo

	for index, expr := range rhs {
		visitor.scan.convert(expr.Pos(), checked.TypeOf(expr), target(index))
	}
}

func (visitor *conversionVisitor) convertTuple(
	rhs ast.Expr,
	count int,
	target func(index int) types.Type,
) {
	tuple, isTuple := visitor.scan.pkg.TypesInfo.TypeOf(rhs).(*types.Tuple)
	if !isTuple {
		return
	}

	for index := range min(tuple.Len(), count) {
		visitor.scan.convert(rhs.Pos(), tuple.At(index).Type(), target(index))
	}
}

func (visitor *conversionVisitor) compare(expr *ast.BinaryExpr) {
	if expr.Op == token.EQL || expr.Op == token.NEQ {
		visitor.exchange(expr.X, expr.Y)
	}
}

func (visitor *conversionVisitor) switchCases(stmt *ast.SwitchStmt) {
	if stmt.Tag == nil {
		return
	}

	for _, clause := range stmt.Body.List {
		if cc, isClause := clause.(*ast.CaseClause); isClause {
			for _, expr := range cc.List {
				visitor.exchange(stmt.Tag, expr)
			}
		}
	}
}

func (visitor *conversionVisitor) exchange(left, right ast.Expr) {
	checked := visitor.scan.pkg.TypesInfo

	// Comparing converts the concrete side to the interface one; demand ignores the rest.
	// The temporary interface never escapes, so only the demand counts.
	visitor.scan.demand(left.Pos(), checked.TypeOf(left), checked.TypeOf(right))
	visitor.scan.demand(right.Pos(), checked.TypeOf(right), checked.TypeOf(left))
}

func (visitor *conversionVisitor) composite(lit *ast.CompositeLit) {
	checked := visitor.scan.pkg.TypesInfo

	literal := checked.TypeOf(lit)
	if literal == nil {
		return
	}

	if pointer, isPointer := literal.Underlying().(*types.Pointer); isPointer {
		literal = pointer.Elem()
	}

	switch shape := literal.Underlying().(type) {
	case *types.Struct:
		for index, elt := range lit.Elts {
			field, value := structElement(shape, index, elt)
			if field != nil {
				visitor.scan.convert(value.Pos(), checked.TypeOf(value), field.Type())
			}
		}
	case *types.Slice:
		visitor.elements(lit, nil, shape.Elem())
	case *types.Array:
		visitor.elements(lit, nil, shape.Elem())
	case *types.Map:
		visitor.elements(lit, shape.Key(), shape.Elem())
	}
}

func (visitor *conversionVisitor) elements(lit *ast.CompositeLit, key, elem types.Type) {
	checked := visitor.scan.pkg.TypesInfo

	for _, elt := range lit.Elts {
		pair, isPair := elt.(*ast.KeyValueExpr)
		if !isPair {
			visitor.scan.convert(elt.Pos(), checked.TypeOf(elt), elem)

			continue
		}

		visitor.scan.convert(pair.Value.Pos(), checked.TypeOf(pair.Value), elem)

		if key != nil {
			visitor.scan.convert(pair.Key.Pos(), checked.TypeOf(pair.Key), key)
		}
	}
}

func structElement(shape *types.Struct, index int, elt ast.Expr) (*types.Var, ast.Expr) {
	pair, isPair := elt.(*ast.KeyValueExpr)
	if !isPair {
		if index >= shape.NumFields() {
			return nil, elt
		}

		return shape.Field(index), elt
	}

	name, isName := pair.Key.(*ast.Ident)
	if !isName {
		return nil, elt
	}

	for field := range shape.Fields() {
		if field.Name() == name.Name {
			return field, pair.Value
		}
	}

	return nil, elt
}

func (visitor *conversionVisitor) typeSwitch(stmt *ast.TypeSwitchStmt) {
	checked := visitor.scan.pkg.TypesInfo

	var assertion ast.Expr

	switch assign := stmt.Assign.(type) {
	case *ast.ExprStmt:
		assertion = assign.X
	case *ast.AssignStmt:
		if len(assign.Rhs) == 1 {
			assertion = assign.Rhs[0]
		}
	}

	asserted, isAssertion := assertion.(*ast.TypeAssertExpr)
	if !isAssertion {
		return
	}

	subject := checked.TypeOf(asserted.X)

	for _, clause := range stmt.Body.List {
		cc, isClause := clause.(*ast.CaseClause)
		if !isClause {
			continue
		}

		for _, expr := range cc.List {
			if checked.Types[expr].IsType() {
				visitor.scan.convert(expr.Pos(), checked.TypeOf(expr), subject)
			}
		}
	}
}
