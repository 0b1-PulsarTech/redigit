package golang

import (
	"go/ast"
	"go/token"
	"go/types"
)

// conversionVisitor walks one file looking for the places Go converts a value
// to an interface without a word: an argument, an assignment, a return, an
// element of a composite literal, a channel send, a type assertion. Each is a
// pair of (the value's type, the type the place expects), read from the
// checker's own record of both.
type conversionVisitor struct {
	scan *factScan
	// results is the result list of the function the walk is inside.
	results *types.Tuple
}

func (v *conversionVisitor) Visit(node ast.Node) ast.Visitor {
	checked := v.scan.pkg.TypesInfo

	switch node := node.(type) {
	case *ast.FuncDecl:
		return v.within(checked.Defs[node.Name])
	case *ast.FuncLit:
		sig, _ := checked.TypeOf(node).(*types.Signature)

		return v.withResults(sig)
	case *ast.CallExpr:
		v.call(node)
	case *ast.AssignStmt:
		v.assign(node)
	case *ast.ValueSpec:
		v.valueSpec(node)
	case *ast.ReturnStmt:
		v.returns(node)
	case *ast.CompositeLit:
		v.composite(node)
	case *ast.SendStmt:
		if ch, isChan := checked.TypeOf(node.Chan).Underlying().(*types.Chan); isChan {
			v.scan.convert(node.Pos(), checked.TypeOf(node.Value), ch.Elem())
		}
	case *ast.TypeAssertExpr:
		if node.Type != nil {
			v.scan.convert(node.Pos(), checked.TypeOf(node.Type), checked.TypeOf(node.X))
		}
	case *ast.TypeSwitchStmt:
		v.typeSwitch(node)
	}

	return v
}

func (v *conversionVisitor) within(obj types.Object) ast.Visitor {
	fn, isFunc := obj.(*types.Func)
	if !isFunc {
		return v
	}

	sig, _ := fn.Type().(*types.Signature)

	return v.withResults(sig)
}

func (v *conversionVisitor) withResults(sig *types.Signature) ast.Visitor {
	if sig == nil {
		return &conversionVisitor{scan: v.scan}
	}

	return &conversionVisitor{scan: v.scan, results: sig.Results()}
}

func (v *conversionVisitor) call(call *ast.CallExpr) {
	checked := v.scan.pkg.TypesInfo
	fun := checked.Types[call.Fun]

	if fun.IsType() {
		if len(call.Args) == 1 {
			v.scan.convert(call.Pos(), checked.TypeOf(call.Args[0]), fun.Type)
		}

		return
	}

	sig, isSig := fun.Type.Underlying().(*types.Signature)
	if !isSig || fun.IsBuiltin() {
		return
	}

	params := sig.Params()

	for index, arg := range call.Args {
		v.scan.convert(arg.Pos(), checked.TypeOf(arg), paramAt(sig, params, index, call.Ellipsis))
	}
}

// paramAt is the type an argument at index is passed as. Past the last
// parameter of a variadic function it is the element type, unless the call
// spreads a slice with "...".
func paramAt(sig *types.Signature, params *types.Tuple, index int, ellipsis token.Pos) types.Type {
	last := params.Len() - 1
	if last < 0 {
		return nil
	}

	if !sig.Variadic() || index < last {
		if index > last {
			return nil
		}

		return params.At(index).Type()
	}

	if ellipsis.IsValid() {
		return nil
	}

	if slice, isSlice := params.At(last).Type().(*types.Slice); isSlice {
		return slice.Elem()
	}

	return nil
}

func (v *conversionVisitor) assign(stmt *ast.AssignStmt) {
	if stmt.Tok != token.ASSIGN || len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}

	checked := v.scan.pkg.TypesInfo

	for index, target := range stmt.Lhs {
		v.scan.convert(
			stmt.Rhs[index].Pos(),
			checked.TypeOf(stmt.Rhs[index]),
			checked.TypeOf(target),
		)
	}
}

func (v *conversionVisitor) valueSpec(spec *ast.ValueSpec) {
	if spec.Type == nil || len(spec.Values) != len(spec.Names) {
		return
	}

	checked := v.scan.pkg.TypesInfo
	target := checked.TypeOf(spec.Type)

	for _, value := range spec.Values {
		v.scan.convert(value.Pos(), checked.TypeOf(value), target)
	}
}

func (v *conversionVisitor) returns(stmt *ast.ReturnStmt) {
	if v.results == nil || len(stmt.Results) != v.results.Len() {
		return
	}

	checked := v.scan.pkg.TypesInfo

	for index, result := range stmt.Results {
		v.scan.convert(result.Pos(), checked.TypeOf(result), v.results.At(index).Type())
	}
}
