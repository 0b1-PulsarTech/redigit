package syntax

import (
	"go/ast"
	"go/token"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func buildFuncDecl(
	fset *token.FileSet,
	path string,
	source []byte,
	unit entities.UnitID,
	isTestFile bool,
	decl *ast.FuncDecl,
) entities.Decl {
	kind := entities.KindFunc
	name := decl.Name.Name

	if decl.Recv != nil {
		kind = entities.KindMethod

		if recv := receiverName(decl.Recv); recv != "" {
			name = recv + "." + name
		}
	}

	start := decl.Pos()
	if decl.Doc != nil {
		start = decl.Doc.Pos()
	}

	return entities.Decl{
		Key:      entities.DeclKey{File: path, Kind: kind, Name: name},
		Unit:     unit,
		Span:     spanOf(fset, start, decl.End()),
		Selector: LocationOf(fset, path, source, decl.Name.Pos()),
		IsTest:   isTestFile && decl.Recv == nil && isTestName(decl.Name.Name),
	}
}

func receiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	// Strip pointer and generics: otherwise two types that implement Error() collapse into one decl.
	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.IndexExpr:
		return receiverBase(typed.X)
	case *ast.IndexListExpr:
		return receiverBase(typed.X)
	default:
		return ""
	}
}

func receiverBase(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}

	return ""
}

func isTestName(name string) bool {
	for _, prefix := range [...]string{"Test", "Benchmark", "Fuzz", "Example"} {
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		rest := name[len(prefix):]
		if rest == "" {
			return true
		}

		first, _ := utf8.DecodeRuneInString(rest)
		if !unicode.IsLower(first) {
			return true
		}
	}

	return false
}

type Ident struct {
	Kind entities.DeclKind
	Name string
}

func FuncIdent(fn *ast.FuncDecl) Ident {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return Ident{entities.KindFunc, fn.Name.Name}
	}

	return Ident{entities.KindMethod, receiverName(fn.Recv) + "." + fn.Name.Name}
}
