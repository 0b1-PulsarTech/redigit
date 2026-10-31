package typed

import (
	"go/ast"
	"go/token"
	"go/types"
	"math"
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
	case *ast.BinaryExpr:
		visitor.compare(typed)
	case *ast.SwitchStmt:
		visitor.switchCases(typed)
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

	if fun.Type == nil {
		return
	}

	// A builtin records its call-site signature too: append's variadic element is a conversion.
	sig, isSig := fun.Type.Underlying().(*types.Signature)
	if !isSig {
		return
	}

	params := sig.Params()

	if len(call.Args) == 1 {
		visitor.convertTuple(call.Args[0], math.MaxInt, func(index int) types.Type {
			return paramAt(sig, params, index, call.Ellipsis)
		})
	}

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
	if stmt.Tok != token.ASSIGN {
		return
	}

	visitor.convertEach(stmt.Rhs, len(stmt.Lhs), func(index int) types.Type {
		return visitor.scan.pkg.TypesInfo.TypeOf(stmt.Lhs[index])
	})
}

func (visitor *conversionVisitor) valueSpec(spec *ast.ValueSpec) {
	if spec.Type == nil {
		return
	}

	target := visitor.scan.pkg.TypesInfo.TypeOf(spec.Type)

	visitor.convertEach(spec.Values, len(spec.Names), func(int) types.Type { return target })
}

func (visitor *conversionVisitor) returns(stmt *ast.ReturnStmt) {
	if visitor.results == nil {
		return
	}

	visitor.convertEach(stmt.Results, visitor.results.Len(), func(index int) types.Type {
		return visitor.results.At(index).Type()
	})
}
