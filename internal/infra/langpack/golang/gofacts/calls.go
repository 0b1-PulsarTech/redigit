package gofacts

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type walker struct {
	refs  refs
	info  *types.Info
	index *Index
}

func (index *Index) walk(pkgs []*packages.Package, where typed.Positions) {
	seen := map[string]bool{}

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		scan := walker{refs: refs{where: where, fset: pkg.Fset}, info: pkg.TypesInfo, index: index}

		for _, file := range pkg.Syntax {
			// A test variant repeats the files of its package, and each is read once.
			name := pkg.Fset.Position(file.Pos()).Filename
			if seen[name] {
				continue
			}

			seen[name] = true

			ast.Inspect(file, scan.visit)
		}
	}
}

func (scan walker) visit(node ast.Node) bool {
	switch cur := node.(type) {
	case *ast.FuncDecl:
		scan.ctor(cur)
	case *ast.CallExpr:
		scan.record(cur)
	}

	return true
}

func (scan walker) record(expr *ast.CallExpr) {
	fn, ident := scan.callee(expr.Fun)
	// A call through a func-typed variable, parameter or field has no *types.Func: its target is
	// only known at run time, so it yields no Call.
	if fn == nil {
		return
	}

	site, inTree := scan.refs.where.DeclAt(scan.refs.fset, expr.Pos())
	if !inTree {
		return
	}

	recv, hasRecv := receiverOf(fn)
	// A method of an interface literal has no declaring type to name; its bare name would read as a
	// package func, so a missing fact is better than that false one.
	if hasRecv && recv == nil {
		return
	}

	typeArgs := scan.receiverArgs(recv)
	fn = fn.Origin()
	call := facts.Call{Site: site, Callee: fn.Name(), TypeArgs: typeArgs}
	call.CalleeKey = scan.funcKey(fn)

	// The receiver is the type declaring the method, not the static type of the operand: a call
	// through an embedding interface or struct names the embedded type.
	if recv != nil {
		call.Receiver = scan.refs.receiver(recv.Obj())
		call.Callee = recv.Obj().Name() + "." + call.Callee
	}

	if fn.Pkg() != nil {
		call.Callee = fn.Pkg().Path() + "." + call.Callee
	}

	if instance, generic := scan.info.Instances[ident]; generic {
		for arg := range instance.TypeArgs.Types() {
			call.TypeArgs = append(call.TypeArgs, scan.refs.ref(arg))
		}
	}

	scan.arguments(&call, expr.Args)
	scan.index.calls[site] = append(scan.index.calls[site], call)
}

func (scan walker) arguments(into *facts.Call, args []ast.Expr) {
	for _, arg := range args {
		if fn := scan.funcValue(arg); fn != nil {
			if key := scan.funcKey(fn.Origin()); key != (entities.DeclKey{}) {
				into.FuncArgs = append(into.FuncArgs, key)
			}
		}

		if value := scan.info.Types[arg].Value; value != nil && value.Kind() == constant.String {
			into.Literals = append(into.Literals, constant.StringVal(value))
		}
	}
}

func (scan walker) callee(fun ast.Expr) (*types.Func, *ast.Ident) {
	for {
		switch cur := fun.(type) {
		case *ast.ParenExpr:
			fun = cur.X
		case *ast.IndexExpr:
			fun = cur.X
		case *ast.IndexListExpr:
			fun = cur.X
		case *ast.SelectorExpr:
			fn, _ := scan.info.Uses[cur.Sel].(*types.Func)

			return fn, cur.Sel
		case *ast.Ident:
			fn, _ := scan.info.Uses[cur].(*types.Func)

			return fn, cur
		default:
			return nil, nil
		}
	}
}

func (scan walker) funcKey(fn *types.Func) entities.DeclKey {
	// An interface method has no body to point at; DeclAt would answer its type or enclosing func.
	if sig, isSig := fn.Type().(*types.Signature); isSig && sig.Recv() != nil &&
		types.IsInterface(sig.Recv().Type()) {
		return entities.DeclKey{}
	}

	key, _ := scan.refs.where.ObjectAt(scan.refs.fset, fn)
	if key.Kind != entities.KindFunc && key.Kind != entities.KindMethod {
		return entities.DeclKey{}
	}

	return key
}

func (scan walker) funcValue(arg ast.Expr) *types.Func {
	fn, _ := scan.callee(arg)

	return fn
}
