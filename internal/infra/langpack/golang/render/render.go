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

// Render writes source back holding only the declarations named in keep.
//
// github.com/dave/dst/decorator would be the natural tool here - it attaches
// comments to the node they document instead of to the file's flat comment
// list, which is exactly what go/ast.FilterFile does NOT do. It cannot be
// imported in this module as it stands: decorator.go unconditionally imports
// golang.org/x/tools/go/packages, which go.mod does not require, and this
// package may not edit go.mod. This renders with go/ast + go/printer instead,
// using ast.NewCommentMap to re-associate comments with the nodes that
// survive filtering - the same fix dst applies, done by hand: a comment
// travels with the node CommentMap attached it to, so it is dropped when that
// node is dropped and kept, in place, when it survives.
func Render(_ context.Context, source []byte, keep []entities.DeclKey) ([]byte, error) {
	keepSet := make(map[syntax.Ident]bool, len(keep))
	for _, k := range keep {
		keepSet[syntax.Ident{Kind: k.Kind, Name: k.Name}] = true
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

	return buf.b, nil
}

// bufWriter is an io.Writer that needs no import beyond what this file
// already carries; bytes.Buffer would be a one-import trade for the same
// three lines.
type bufWriter struct{ b []byte }

func (w *bufWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)

	return len(p), nil
}

func isImportDecl(decl ast.Decl) bool {
	gen, ok := decl.(*ast.GenDecl)

	return ok && gen.Tok == token.IMPORT
}

// filterDecl keeps a func/type/const/var declaration whole or drops it whole,
// splitting a const/var/type group into the specs the caller kept.
func filterDecl(
	fset *token.FileSet,
	source []byte,
	decl ast.Decl,
	keepSet map[syntax.Ident]bool,
) []ast.Decl {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if keepSet[syntax.FuncIdent(d)] {
			keepStatements(d, keepSet)

			return []ast.Decl{d}
		}

		return nil
	case *ast.GenDecl:
		return filterGenDecl(fset, source, d, keepSet)
	default:
		return nil
	}
}

// keepStatements cuts a function down to the statements the caller kept. A
// function none of whose statements is named was not cut when the declarations
// were read, so it is written whole.
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
