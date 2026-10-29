package declsource

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// ReturnedConcreteTypes parses a function's own text and reports the local
// types it constructs and returns, only when a result type is a known
// interface. That gate keeps this from waiting on every construction in the
// repository: returning a *T where the signature says *T converts nothing.
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

// ParseFunc reads one function declaration out of its own source text.
//
// No check that the text starts with "func": a declaration's span carries its
// doc comment, and real code comments almost everything. Requiring the keyword
// first silently skipped every documented constructor, which is all of them.
func ParseFunc(text string) *ast.FuncDecl {
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

// resultsHoldInterface reports whether any result type names a known
// interface. A type argument counts too: "Rule[string]" is Rule.
func resultsHoldInterface(signature *ast.FuncType, interfaces map[string]bool) bool {
	if signature == nil || signature.Results == nil {
		return false
	}

	for _, result := range signature.Results.List {
		if name, found := TypeName(result.Type); found && interfaces[name] {
			return true
		}
	}

	return false
}

// TypeName reduces a type expression to the bare name it rests on:
// "ruletypes.Rule[string]" and "*Rule" both reduce to Rule.
func TypeName(expr ast.Expr) (string, bool) {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name, true
	case *ast.StarExpr:
		return TypeName(node.X)
	case *ast.ParenExpr:
		return TypeName(node.X)
	case *ast.IndexExpr:
		return TypeName(node.X)
	case *ast.IndexListExpr:
		return TypeName(node.X)
	case *ast.SelectorExpr:
		return node.Sel.Name, true
	default:
		return "", false
	}
}

// ConcreteTypeIn reports the local type an expression names where an interface
// is expected, in every form: "(*T)(nil)", "T(nil)", "&T{}", "T{}".
// Enumerating them one at a time let "var _ I = T{}" through and cost 300 of
// 327 failing trees - a value literal is a composite, not a conversion.
func ConcreteTypeIn(expr ast.Expr) (string, bool) {
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

// localTypeName is typeName restricted to a type THIS package declares: a
// qualified one has no methods in this unit. It unwraps before deciding,
// because "(*other.Thing)(nil)" hides its qualifier two levels down and
// testing the outermost expression let it through as "Thing".
func localTypeName(expr ast.Expr) (string, bool) {
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

// ParseDeclaration reads any top-level declaration out of its own source text,
// doc comment and all.
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
