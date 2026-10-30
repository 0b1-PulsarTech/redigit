package gofacts

import (
	"go/ast"
	"go/types"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func (scan walker) ctor(decl *ast.FuncDecl) {
	fn, isFunc := scan.info.Defs[decl.Name].(*types.Func)
	if !isFunc {
		return
	}

	key, inTree := scan.refs.where.DeclAt(scan.refs.fset, decl.Name.Pos())
	if !inTree || (key.Kind != entities.KindFunc && key.Kind != entities.KindMethod) {
		return
	}

	// The same function seen through a test variant keeps the first answer.
	if _, known := scan.index.ctors[key]; known {
		return
	}

	params := fn.Signature().Params()
	taken := make([]facts.TypeRef, 0, params.Len())

	for param := range params.Variables() {
		taken = append(taken, scan.refs.ref(param.Type()))
	}

	scan.index.ctors[key] = taken
}
