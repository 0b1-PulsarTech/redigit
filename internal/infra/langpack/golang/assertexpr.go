package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// receiverOfAssertion parses "_ I = (*T)(nil)" and returns T. The text is
// accepted with or without its own "var", because a declaration's span carries
// the keyword and its name does not.
func receiverOfAssertion(text string) (string, bool) {
	body := strings.TrimSpace(text)
	if !strings.HasPrefix(body, "var ") {
		body = "var " + body
	}

	file, err := parser.ParseFile(
		token.NewFileSet(), "assert.go", "package p\n"+body+"\n", parser.SkipObjectResolution,
	)
	if err != nil {
		return "", false
	}

	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			// The declared TYPE is what makes this an assertion. Without it,
			// "_ = compute()" is a blank assignment that names a function and
			// asserts nothing.
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || value.Type == nil || len(value.Values) != 1 {
				continue
			}

			if name, found := concreteName(value.Values[0]); found {
				return name, true
			}
		}
	}

	return "", false
}

// concreteName digs the type name out of a conversion: "(*T)(nil)", "T(nil)",
// and "(*T[A])(nil)" all name T.
func concreteName(expr ast.Expr) (string, bool) {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return "", false
	}

	return identOf(call.Fun)
}

func identOf(expr ast.Expr) (string, bool) {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name, true
	case *ast.ParenExpr:
		return identOf(node.X)
	case *ast.StarExpr:
		return identOf(node.X)
	case *ast.IndexExpr:
		return identOf(node.X)
	case *ast.IndexListExpr:
		return identOf(node.X)
	case *ast.SelectorExpr:
		// A type from another package: its methods are not in this graph's
		// unit, so nothing here to tie.
		return "", false
	default:
		return "", false
	}
}
