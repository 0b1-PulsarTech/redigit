package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// returnedConcreteTypes parses a function's own text and reports the local
// types it constructs and returns, only when a result type is a known
// interface. That gate keeps this from waiting on every construction in the
// repository: returning a *T where the signature says *T converts nothing.
func returnedConcreteTypes(text string, interfaces map[string]bool) []string {
	fn := parseFunc(text)
	if fn == nil || fn.Body == nil || !resultsHoldInterface(fn.Type, interfaces) {
		return nil
	}

	seen := map[string]bool{}

	var out []string

	ast.Inspect(fn.Body, func(node ast.Node) bool {
		ret, isReturn := node.(*ast.ReturnStmt)
		if !isReturn {
			return true
		}

		for _, value := range ret.Results {
			if name, built := constructedName(value); built && !seen[name] {
				seen[name] = true

				out = append(out, name)
			}
		}

		return true
	})

	return out
}

// parseFunc reads one function declaration out of its own source text.
func parseFunc(text string) *ast.FuncDecl {
	body := strings.TrimSpace(text)
	if !strings.HasPrefix(body, "func") {
		return nil
	}

	file, err := parser.ParseFile(
		token.NewFileSet(), "fn.go", "package p\n"+body+"\n", parser.SkipObjectResolution,
	)
	if err != nil {
		return nil
	}

	for _, decl := range file.Decls {
		if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
			return fn
		}
	}

	return nil
}

// resultsHoldInterface reports whether any result type names a known
// interface. A type argument counts too: "Rule[string]" is Rule.
func resultsHoldInterface(signature *ast.FuncType, interfaces map[string]bool) bool {
	if signature == nil || signature.Results == nil {
		return false
	}

	for _, result := range signature.Results.List {
		if name, found := typeName(result.Type); found && interfaces[name] {
			return true
		}
	}

	return false
}

// typeName reduces a type expression to the bare name it rests on:
// "ruletypes.Rule[string]" and "*Rule" both reduce to Rule.
func typeName(expr ast.Expr) (string, bool) {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name, true
	case *ast.StarExpr:
		return typeName(node.X)
	case *ast.ParenExpr:
		return typeName(node.X)
	case *ast.IndexExpr:
		return typeName(node.X)
	case *ast.IndexListExpr:
		return typeName(node.X)
	case *ast.SelectorExpr:
		return node.Sel.Name, true
	default:
		return "", false
	}
}

// constructedName reports the type a returned expression builds: "&T{}",
// "T{}", and "newT()" are three different things, and only the first two name
// a type this file constructs directly.
func constructedName(expr ast.Expr) (string, bool) {
	switch node := expr.(type) {
	case *ast.UnaryExpr:
		if node.Op == token.AND {
			return constructedName(node.X)
		}

		return "", false
	case *ast.CompositeLit:
		if node.Type == nil {
			return "", false
		}

		return localTypeName(node.Type)
	default:
		return "", false
	}
}

// localTypeName is typeName restricted to a type this package declares: a
// qualified one belongs to another package, whose methods are not in this
// unit.
func localTypeName(expr ast.Expr) (string, bool) {
	if _, qualified := expr.(*ast.SelectorExpr); qualified {
		return "", false
	}

	return typeName(expr)
}
