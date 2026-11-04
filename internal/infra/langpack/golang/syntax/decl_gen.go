package syntax

import (
	"go/ast"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func buildGenDecl(
	fset *token.FileSet,
	source []byte,
	path string,
	unit entities.UnitID,
	decl *ast.GenDecl,
) []entities.Decl {
	// Identity is one key per name, but the rendering unit is the whole iota group.
	switch decl.Tok {
	case token.IMPORT:
		return nil
	case token.TYPE:
		return buildTypeSpecs(fset, path, source, unit, decl)
	case token.CONST:
		if NeedsItsBlock(decl) {
			return buildAtomicGroup(fset, path, source, unit, decl)
		}

		return buildValueSpecs(fset, source, path, unit, decl)
	case token.VAR:
		return buildValueSpecs(fset, source, path, unit, decl)
	default:
		return nil
	}
}

func buildTypeSpecs(
	fset *token.FileSet,
	path string,
	source []byte,
	unit entities.UnitID,
	decl *ast.GenDecl,
) []entities.Decl {
	var decls []entities.Decl

	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		start := specStart(decl, ts.Doc, ts.Pos())

		_, isInterface := ts.Type.(*ast.InterfaceType)

		decls = append(decls, entities.Decl{
			Key:         entities.DeclKey{File: path, Kind: entities.KindType, Name: ts.Name.Name},
			Unit:        unit,
			Span:        spanOf(fset, start, ts.End()),
			Selector:    LocationOf(fset, path, source, ts.Name.Pos()),
			IsInterface: isInterface,
		})
	}

	return decls
}

func buildValueSpecs(
	fset *token.FileSet,
	source []byte,
	path string,
	unit entities.UnitID,
	decl *ast.GenDecl,
) []entities.Decl {
	var decls []entities.Decl

	// A grouped block is one thought even when it binds two names: an enum of
	// two members is as indivisible as one of twenty-seven. Counting siblings
	// per FILE gets that wrong the moment a file holds two enums, or one enum
	// beside a loose constant.
	size := len(GroupNames(decl))

	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		start := specStart(decl, vs.Doc, vs.Pos())

		// The renderer keeps a whole spec when any of its names is planned, so its names must be born together.
		atom := ""
		if len(vs.Names) > 1 {
			atom = atomLabel("spec-group", fset, vs.Pos())
		}

		for _, name := range vs.Names {
			decls = append(decls, entities.Decl{
				Atom: atom,
				Key: entities.DeclKey{
					File: path,
					Kind: entities.KindValue,
					Name: ValueName(fset, source, vs, name),
				},
				Unit:      unit,
				Span:      spanOf(fset, start, vs.End()),
				Selector:  LocationOf(fset, path, source, name.Pos()),
				GroupSize: size,
			})
		}
	}

	return decls
}

func specStart(decl *ast.GenDecl, specDoc *ast.CommentGroup, specPos token.Pos) token.Pos {
	if specDoc != nil {
		return specDoc.Pos()
	}

	if !decl.Lparen.IsValid() && decl.Doc != nil {
		return decl.Doc.Pos()
	}

	return specPos
}

func ValueName(fset *token.FileSet, source []byte, vs *ast.ValueSpec, name *ast.Ident) string {
	if name.Name != "_" {
		return name.Name
	}

	// Blank names are told apart by type plus value: every "var _ Iface = (*T)(nil)" shares one Iface.
	text := ""

	if vs.Type != nil {
		text += exprText(fset, source, vs.Type)
	}

	if len(vs.Values) > 0 {
		text += " = " + exprText(fset, source, vs.Values[0])
	}

	if text == "" {
		return name.Name
	}

	return "_ " + text
}

func exprText(fset *token.FileSet, source []byte, expr ast.Expr) string {
	start := fset.Position(expr.Pos()).Offset
	end := fset.Position(expr.End()).Offset

	return string(source[start:end])
}

func SpecDoc(spec ast.Spec) *ast.CommentGroup {
	switch typed := spec.(type) {
	case *ast.ValueSpec:
		return typed.Doc
	case *ast.TypeSpec:
		return typed.Doc
	case *ast.ImportSpec:
		return typed.Doc
	default:
		return nil
	}
}
