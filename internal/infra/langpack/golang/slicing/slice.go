package slicing

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

const (
	minStatements = 2
)

func Slice(
	_ context.Context,
	fn entities.Decl,
	source []byte,
	policy sliceconf.Config,
) (indexing.Sliced, bool, error) {
	if fn.Key.Kind != entities.KindFunc && fn.Key.Kind != entities.KindMethod {
		return indexing.Sliced{}, false, nil
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, fn.Key.File, source, parser.ParseComments)
	if err != nil {
		return indexing.Sliced{}, false, fmt.Errorf("parse %s to slice: %w", fn.Key.File, err)
	}

	decl := findFuncDecl(file, fn.Key)
	if decl == nil || decl.Body == nil {
		return indexing.Sliced{}, false, nil
	}

	cut := bodyCut{fset: fset, fn: fn, decl: decl, stmts: decl.Body.List, policy: policy}

	// A registration table that grows one element per feature is cut by element.
	if elements, _ := syntax.ReturnedElements(decl.Body); len(elements) >= syntax.MinGroup {
		cut.elements = elements

		return cut.sliceElements(), true, nil
	}

	if !cuttable(decl.Body) {
		return indexing.Sliced{}, false, nil
	}

	cut.comments = ast.NewCommentMap(fset, file, file.Comments)
	cut.info = typeLocals(fset, file)

	return cut.slice(), true, nil
}

func findFuncDecl(file *ast.File, key entities.DeclKey) *ast.FuncDecl {
	var found *ast.FuncDecl

	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			continue
		}

		ident := syntax.FuncIdent(fn)
		if ident.Kind != key.Kind || ident.Name != key.Name {
			continue
		}

		// Two functions share a key (init, or _), and the statements of one would be
		// filtered out of both, so neither is cut.
		if found != nil {
			return nil
		}

		found = fn
	}

	return found
}

func cuttable(body *ast.BlockStmt) bool {
	if len(body.List) < minStatements {
		return false
	}

	// An early return leaves what follows unreachable only when all of it is present.
	for _, stmt := range body.List[:len(body.List)-1] {
		if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
			return false
		}
	}

	// A label or goto ties statements together by position, so such a body is not cut.
	tied := false

	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.LabeledStmt:
			tied = true
		case *ast.BranchStmt:
			tied = tied || typed.Tok == token.GOTO
		}

		return !tied
	})

	return !tied
}
