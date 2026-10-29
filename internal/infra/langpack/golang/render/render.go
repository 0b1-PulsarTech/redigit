package render

import (
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/genmark"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

func Render(_ context.Context, source []byte, keep []entities.DeclKey) ([]byte, error) {
	keepSet := make(map[syntax.Ident]bool, len(keep))
	for _, key := range keep {
		keepSet[syntax.Ident{Kind: key.Kind, Name: key.Name}] = true
	}

	// A file that declares nothing is carried by one declaration standing for
	// the whole of it, so keeping that one means keeping the file verbatim.
	// Filtering it through the printer instead would drop its imports, which
	// are the only thing it holds.
	if keepSet[syntax.Ident{Kind: entities.KindOther, Name: syntax.FileDeclName}] {
		return source, nil
	}

	// A generated file is reproduced verbatim: its declarations are one atom
	// and arrive together, and filtering through the printer would drop the
	// marker comment that says a tool wrote it.
	if len(keep) > 0 && genmark.IsGenerated(source) && !genmark.IsSqlcOutput(source) {
		return source, nil
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(
		fset,
		"",
		source,
		parser.ParseComments,
	)
	if err != nil {
		return nil, fmt.Errorf("parse source for render: %w", err)
	}

	cmap := ast.NewCommentMap(fset, file, file.Comments)

	var kept, importDecls []ast.Decl

	for _, decl := range file.Decls {
		if isImportDecl(decl) {
			importDecls = append(importDecls, decl)

			continue
		}

		kept = append(kept, filterDecl(fset, source, decl, keepSet)...)
	}

	if len(kept) == 0 {
		return []byte{}, nil
	}

	imports := filterImports(fset, importDecls, usedIdents(kept))
	file.Decls = append(imports, kept...)
	file.Comments = cmap.Filter(file).Comments()

	var buf bufWriter

	if err := format.Node(&buf, fset, file); err != nil {
		return nil, fmt.Errorf("render source: %w", err)
	}

	return buf.buf, nil
}

type bufWriter struct{ buf []byte }

func (writer *bufWriter) Write(payload []byte) (int, error) {
	writer.buf = append(writer.buf, payload...)

	return len(payload), nil
}

func isImportDecl(decl ast.Decl) bool {
	gen, ok := decl.(*ast.GenDecl)

	return ok && gen.Tok == token.IMPORT
}

func filterDecl(
	fset *token.FileSet,
	source []byte,
	decl ast.Decl,
	keepSet map[syntax.Ident]bool,
) []ast.Decl {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		if keepSet[syntax.FuncIdent(typed)] {
			keepStatements(typed, keepSet)

			return []ast.Decl{typed}
		}

		return nil
	case *ast.GenDecl:
		return filterGenDecl(fset, source, typed, keepSet)
	default:
		return nil
	}
}

func keepStatements(fn *ast.FuncDecl, keepSet map[syntax.Ident]bool) {
	if fn.Body == nil {
		return
	}

	name := syntax.FuncIdent(fn).Name

	if elements, replace := syntax.ReturnedElements(fn.Body); len(elements) >= syntax.MinGroup {
		keptElements := make([]ast.Expr, 0, len(elements))

		for index, element := range elements {
			if keepSet[syntax.Ident{Kind: entities.KindStmt, Name: syntax.StmtName(name, index)}] {
				keptElements = append(keptElements, element)
			}
		}

		if len(keptElements) > 0 {
			replace(keptElements)
		}

		return
	}

	kept := make([]ast.Stmt, 0, len(fn.Body.List))

	for index, stmt := range fn.Body.List {
		if keepSet[syntax.Ident{Kind: entities.KindStmt, Name: syntax.StmtName(name, index)}] {
			kept = append(kept, stmt)
		}
	}

	if len(kept) > 0 {
		fn.Body.List = kept
	}
}
