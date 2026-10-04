package golang

import (
	"go/ast"
	"go/types"
)

// composite reads the elements of a literal against the type each position of
// the literal's own type expects.
func (v *conversionVisitor) composite(lit *ast.CompositeLit) {
	checked := v.scan.pkg.TypesInfo

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
				v.scan.convert(value.Pos(), checked.TypeOf(value), field.Type())
			}
		}
	case *types.Slice:
		v.elements(lit, nil, shape.Elem())
	case *types.Array:
		v.elements(lit, nil, shape.Elem())
	case *types.Map:
		v.elements(lit, shape.Key(), shape.Elem())
	}
}

func (v *conversionVisitor) elements(lit *ast.CompositeLit, key, elem types.Type) {
	checked := v.scan.pkg.TypesInfo

	for _, elt := range lit.Elts {
		pair, isPair := elt.(*ast.KeyValueExpr)
		if !isPair {
			v.scan.convert(elt.Pos(), checked.TypeOf(elt), elem)

			continue
		}

		v.scan.convert(pair.Value.Pos(), checked.TypeOf(pair.Value), elem)

		if key != nil {
			v.scan.convert(pair.Key.Pos(), checked.TypeOf(pair.Key), key)
		}
	}
}

// structElement is the field a literal element initialises and the expression
// that initialises it, by key when the element has one and by position when not.
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

// typeSwitch reads each case type against the interface the switch inspects.
func (v *conversionVisitor) typeSwitch(stmt *ast.TypeSwitchStmt) {
	checked := v.scan.pkg.TypesInfo

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
				v.scan.convert(expr.Pos(), checked.TypeOf(expr), subject)
			}
		}
	}
}
