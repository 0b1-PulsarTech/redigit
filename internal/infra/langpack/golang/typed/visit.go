package typed

import (
	"go/ast"
	"go/token"
	"go/types"
)

type conversionVisitor struct {
	scan *factScan
	// results is the result list of the function the walk is inside.
	results *types.Tuple
}

func (visitor *conversionVisitor) Visit(node ast.Node) ast.Visitor {
	checked := visitor.scan.pkg.TypesInfo

	switch typed := node.(type) {
	case *ast.FuncDecl:
		return visitor.within(checked.Defs[typed.Name])
	case *ast.FuncLit:
		sig, _ := checked.TypeOf(typed).(*types.Signature)

		return visitor.withResults(sig)
	case *ast.CallExpr:
		visitor.call(typed)
	case *ast.AssignStmt:
		visitor.assign(typed)
	case *ast.ValueSpec:
		visitor.valueSpec(typed)
	case *ast.ReturnStmt:
		visitor.returns(typed)
	case *ast.CompositeLit:
		visitor.composite(typed)
	case *ast.SendStmt:
		if ch, isChan := checked.TypeOf(typed.Chan).Underlying().(*types.Chan); isChan {
			visitor.scan.convert(typed.Pos(), checked.TypeOf(typed.Value), ch.Elem())
		}
	case *ast.TypeAssertExpr:
		if typed.Type != nil {
			visitor.scan.convert(typed.Pos(), checked.TypeOf(typed.Type), checked.TypeOf(typed.X))
		}
	case *ast.TypeSwitchStmt:
		visitor.typeSwitch(typed)
	}

	return visitor
}

func (visitor *conversionVisitor) within(obj types.Object) ast.Visitor {
	fn, isFunc := obj.(*types.Func)
	if !isFunc {
		return visitor
	}

	sig, _ := fn.Type().(*types.Signature)

	return visitor.withResults(sig)
}

func (visitor *conversionVisitor) withResults(sig *types.Signature) ast.Visitor {
	if sig == nil {
		return &conversionVisitor{scan: visitor.scan}
	}

	return &conversionVisitor{scan: visitor.scan, results: sig.Results()}
}

func (visitor *conversionVisitor) call(call *ast.CallExpr) {
	checked := visitor.scan.pkg.TypesInfo
	fun := checked.Types[call.Fun]

	if fun.IsType() {
		if len(call.Args) == 1 {
			visitor.scan.convert(call.Pos(), checked.TypeOf(call.Args[0]), fun.Type)
		}

		return
	}

	sig, isSig := fun.Type.Underlying().(*types.Signature)
	if !isSig || fun.IsBuiltin() {
		return
	}

	params := sig.Params()

	for index, arg := range call.Args {
		visitor.scan.convert(
			arg.Pos(),
			checked.TypeOf(arg),
			paramAt(sig, params, index, call.Ellipsis),
		)
	}
}

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

func (visitor *conversionVisitor) assign(stmt *ast.AssignStmt) {
	if stmt.Tok != token.ASSIGN || len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}

	checked := visitor.scan.pkg.TypesInfo

	for index, target := range stmt.Lhs {
		visitor.scan.convert(
			stmt.Rhs[index].Pos(),
			checked.TypeOf(stmt.Rhs[index]),
			checked.TypeOf(target),
		)
	}
}

func (visitor *conversionVisitor) valueSpec(spec *ast.ValueSpec) {
	if spec.Type == nil || len(spec.Values) != len(spec.Names) {
		return
	}

	checked := visitor.scan.pkg.TypesInfo
	target := checked.TypeOf(spec.Type)

	for _, value := range spec.Values {
		visitor.scan.convert(value.Pos(), checked.TypeOf(value), target)
	}
}

func (visitor *conversionVisitor) returns(stmt *ast.ReturnStmt) {
	if visitor.results == nil || len(stmt.Results) != visitor.results.Len() {
		return
	}

	checked := visitor.scan.pkg.TypesInfo

	for index, result := range stmt.Results {
		visitor.scan.convert(result.Pos(), checked.TypeOf(result), visitor.results.At(index).Type())
	}
}
