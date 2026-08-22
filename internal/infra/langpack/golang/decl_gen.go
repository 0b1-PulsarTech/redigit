package golang

import (
	"go/ast"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// buildGenDecl expands a const/var/type declaration into one Decl per bound
// name, except a const group with an iota-implicit spec (no type, no value),
// which stays one atomic Decl: splitting it would change what an untouched
// constant evaluates to.
func buildGenDecl(
	fset *token.FileSet,
	source []byte,
	path string,
	unit entities.UnitID,
	decl *ast.GenDecl,
) []entities.Decl {
	switch decl.Tok {
	case token.IMPORT:
		return nil
	case token.TYPE:
		return buildTypeSpecs(fset, path, source, unit, decl)
	case token.CONST:
		if hasImplicitSpec(decl) {
			return []entities.Decl{
				buildAtomicGroup(fset, path, source, unit, entities.KindValue, decl),
			}
		}

		return buildValueSpecs(fset, source, path, unit, decl)
	case token.VAR:
		return buildValueSpecs(fset, source, path, unit, decl)
	default:
		return nil
	}
}

func hasImplicitSpec(decl *ast.GenDecl) bool {
	for i, spec := range decl.Specs {
		if i == 0 {
			continue
		}

		vs, ok := spec.(*ast.ValueSpec)
		if ok && vs.Type == nil && len(vs.Values) == 0 {
			return true
		}
	}

	return false
}

func buildAtomicGroup(
	fset *token.FileSet,
	path string,
	source []byte,
	unit entities.UnitID,
	kind entities.DeclKind,
	decl *ast.GenDecl,
) entities.Decl {
	name := groupName(decl)
	start := decl.Pos()

	if decl.Doc != nil {
		start = decl.Doc.Pos()
	}

	return entities.Decl{
		Key:      entities.DeclKey{File: path, Kind: kind, Name: name},
		Unit:     unit,
		Span:     spanOf(fset, start, decl.End()),
		Selector: locationOf(fset, path, source, groupNamePos(decl)),
	}
}

func groupName(decl *ast.GenDecl) string {
	if len(decl.Specs) == 0 {
		return ""
	}

	vs, ok := decl.Specs[0].(*ast.ValueSpec)
	if !ok || len(vs.Names) == 0 {
		return ""
	}

	return vs.Names[0].Name
}

func groupNamePos(decl *ast.GenDecl) token.Pos {
	if len(decl.Specs) == 0 {
		return decl.Pos()
	}

	vs, ok := decl.Specs[0].(*ast.ValueSpec)
	if !ok || len(vs.Names) == 0 {
		return decl.Pos()
	}

	return vs.Names[0].Pos()
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

		decls = append(decls, entities.Decl{
			Key:      entities.DeclKey{File: path, Kind: entities.KindType, Name: ts.Name.Name},
			Unit:     unit,
			Span:     spanOf(fset, start, ts.End()),
			Selector: locationOf(fset, path, source, ts.Name.Pos()),
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

	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		start := specStart(decl, vs.Doc, vs.Pos())

		for _, name := range vs.Names {
			decls = append(decls, entities.Decl{
				Key: entities.DeclKey{
					File: path,
					Kind: entities.KindValue,
					Name: valueName(fset, source, vs, name),
				},
				Unit:     unit,
				Span:     spanOf(fset, start, vs.End()),
				Selector: locationOf(fset, path, source, name.Pos()),
			})
		}
	}

	return decls
}

// specStart anchors the span at the doc comment: the spec's own, or the
// enclosing GenDecl's when the group holds a single, unparenthesized spec.
func specStart(decl *ast.GenDecl, specDoc *ast.CommentGroup, specPos token.Pos) token.Pos {
	if specDoc != nil {
		return specDoc.Pos()
	}

	if !decl.Lparen.IsValid() && decl.Doc != nil {
		return decl.Doc.Pos()
	}

	return specPos
}

// valueName disambiguates a blank identifier by appending its declared type
// and initializer text: every "var _ Iface = (*T)(nil)" in a package shares
// the same Iface, so the type alone would still collapse them all into one
// name. The value (the concrete T being asserted) is what tells them apart.
func valueName(fset *token.FileSet, source []byte, vs *ast.ValueSpec, name *ast.Ident) string {
	if name.Name != "_" {
		return name.Name
	}

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
