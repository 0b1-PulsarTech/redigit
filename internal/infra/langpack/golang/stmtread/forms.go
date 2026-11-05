package stmtread

import (
	"go/ast"
	"go/token"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
)

func readStatement(stmt ast.Stmt) linting.Statement {
	switch node := stmt.(type) {
	case *ast.AssignStmt:
		return linting.Statement{Assign: assignForm(node)}
	case *ast.IfStmt:
		return linting.Statement{Checks: node.Init == nil && testsErr(node.Cond)}
	case *ast.DeclStmt:
		return varStatement(node)
	}

	return linting.Statement{}
}

func varStatement(decl *ast.DeclStmt) linting.Statement {
	group, isGen := decl.Decl.(*ast.GenDecl)
	if !isGen || group.Tok != token.VAR {
		return linting.Statement{}
	}

	// var err T binds err and starts no pair; var a, err = f() binds it from a value.
	for _, spec := range group.Specs {
		values, isValue := spec.(*ast.ValueSpec)
		if !isValue || !slices.ContainsFunc(values.Names, func(name *ast.Ident) bool {
			return name.Name == "err"
		}) {
			continue
		}

		if len(values.Values) > 0 {
			return linting.Statement{Assign: linting.FormDefine}
		}

		return linting.Statement{Binds: true}
	}

	return linting.Statement{}
}

func assignForm(assign *ast.AssignStmt) linting.AssignForm {
	if assign.Tok != token.ASSIGN && assign.Tok != token.DEFINE {
		return linting.FormNone
	}

	// Whether := binds err here or reuses it is told from the earlier statements, not here.
	for _, target := range assign.Lhs {
		if isIdent(target, "err") {
			if assign.Tok == token.ASSIGN {
				return linting.FormAssign
			}

			return linting.FormDefine
		}
	}

	return linting.FormNone
}

func testsErr(cond ast.Expr) bool {
	switch node := cond.(type) {
	case *ast.ParenExpr:
		return testsErr(node.X)
	case *ast.BinaryExpr:
		switch node.Op {
		case token.LAND, token.LOR:
			return testsErr(node.X) || testsErr(node.Y)
		case token.NEQ:
			return isIdent(node.X, "err") && isIdent(node.Y, "nil") ||
				isIdent(node.X, "nil") && isIdent(node.Y, "err")
		}
	}

	return false
}

func isIdent(expr ast.Expr, name string) bool {
	ident, isNamed := expr.(*ast.Ident)

	return isNamed && ident.Name == name
}
