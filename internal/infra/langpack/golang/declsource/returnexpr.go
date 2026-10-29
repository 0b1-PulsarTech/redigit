package declsource

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

func ReturnedConcreteTypes(text string, interfaces map[string]bool) []string {
	fn := ParseFunc(text)
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
			if name, built := ConcreteTypeIn(value); built && !seen[name] {
				seen[name] = true

				out = append(out, name)
			}
		}

		return true
	})

	return out
}

func ParseFunc(text string) *ast.FuncDecl {
	// No "func" prefix check: the span carries the doc comment, so most constructors would be skipped.
	file, err := parser.ParseFile(
		token.NewFileSet(), "fn.go", "package p\n"+strings.TrimSpace(text)+"\n",
		parser.SkipObjectResolution,
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

func ConcreteTypeIn(expr ast.Expr) (string, bool) {
	// Handles (*T)(nil), T(nil), T{} and &T{}, generic T[P] included.
	switch node := expr.(type) {
	case *ast.UnaryExpr:
		if node.Op == token.AND {
			return ConcreteTypeIn(node.X)
		}

		return "", false
	case *ast.CompositeLit:
		if node.Type == nil {
			return "", false
		}

		return localTypeName(node.Type)
	case *ast.CallExpr:
		return localTypeName(node.Fun)
	default:
		return "", false
	}
}

func localTypeName(expr ast.Expr) (string, bool) {
	// Unwrap first: "(*other.Thing)(nil)" hides its qualifier, and only a local type has methods here.
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name, true
	case *ast.ParenExpr:
		return localTypeName(node.X)
	case *ast.StarExpr:
		return localTypeName(node.X)
	case *ast.IndexExpr:
		return localTypeName(node.X)
	case *ast.IndexListExpr:
		return localTypeName(node.X)
	default:
		return "", false
	}
}

func ParseDeclaration(text string) ast.Decl {
	file, err := parser.ParseFile(
		token.NewFileSet(), "decl.go", "package p\n"+strings.TrimSpace(text)+"\n",
		parser.SkipObjectResolution,
	)
	if err != nil || len(file.Decls) == 0 {
		return nil
	}

	return file.Decls[0]
}
