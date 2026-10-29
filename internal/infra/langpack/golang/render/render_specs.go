package render

import (
	"go/ast"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

func filterGenDecl(
	fset *token.FileSet,
	source []byte,
	d *ast.GenDecl,
	keepSet map[syntax.Ident]bool,
) []ast.Decl {
	if d.Tok == token.TYPE {
		return filterSpecs(d, keepSet, func(spec ast.Spec) (syntax.Ident, bool) {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				return syntax.Ident{}, false
			}

			return syntax.Ident{Kind: entities.KindType, Name: ts.Name.Name}, true
		})
	}

	if d.Tok == token.CONST && syntax.HasImplicitSpec(d) {
		// Any surviving name keeps the whole group: an iota constant that is
		// not written down still takes its value from its position in the
		// block, so the block cannot be cut.
		for _, name := range syntax.GroupNames(d) {
			if keepSet[syntax.Ident{Kind: entities.KindValue, Name: name}] {
				return []ast.Decl{d}
			}
		}

		return nil
	}

	return filterValueSpecs(fset, source, d, keepSet)
}

func filterSpecs(
	d *ast.GenDecl,
	keepSet map[syntax.Ident]bool,
	identOf func(ast.Spec) (syntax.Ident, bool),
) []ast.Decl {
	var specs []ast.Spec

	for _, spec := range d.Specs {
		ident, ok := identOf(spec)
		if ok && keepSet[ident] {
			specs = append(specs, spec)
		}
	}

	return finishGenDecl(d, specs)
}

func filterValueSpecs(
	fset *token.FileSet,
	source []byte,
	d *ast.GenDecl,
	keepSet map[syntax.Ident]bool,
) []ast.Decl {
	var specs []ast.Spec

	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, name := range vs.Names {
			if keepSet[syntax.Ident{Kind: entities.KindValue, Name: syntax.ValueName(fset, source, vs, name)}] {
				specs = append(specs, spec)

				break
			}
		}
	}

	return finishGenDecl(d, specs)
}

func finishGenDecl(d *ast.GenDecl, specs []ast.Spec) []ast.Decl {
	if len(specs) == 0 {
		return nil
	}

	if len(specs) < len(d.Specs) {
		resetSpacingIfSafe(specs)
	}

	d.Specs = specs

	return []ast.Decl{d}
}

func resetSpacingIfSafe(specs []ast.Spec) {
	for _, spec := range specs {
		if syntax.SpecDoc(spec) != nil {
			return
		}
	}

	for _, spec := range specs {
		resetSpecPos(spec)
	}
}

func resetSpecPos(spec ast.Spec) {
	switch s := spec.(type) {
	case *ast.ValueSpec:
		for _, name := range s.Names {
			name.NamePos = token.NoPos
		}
	case *ast.TypeSpec:
		s.Name.NamePos = token.NoPos
	case *ast.ImportSpec:
		s.Path.ValuePos = token.NoPos
		s.EndPos = token.NoPos

		if s.Name != nil {
			s.Name.NamePos = token.NoPos
		}
	}
}
