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
	gen *ast.GenDecl,
	keepSet map[syntax.Ident]bool,
) []ast.Decl {
	if gen.Tok == token.TYPE {
		return filterSpecs(gen, keepSet, func(spec ast.Spec) (syntax.Ident, bool) {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				return syntax.Ident{}, false
			}

			return syntax.Ident{Kind: entities.KindType, Name: ts.Name.Name}, true
		})
	}

	if gen.Tok == token.CONST && syntax.HasImplicitSpec(gen) {
		// Any surviving name keeps the whole group: an iota constant that is
		// not written down still takes its value from its position in the
		// block, so the block cannot be cut.
		for _, name := range syntax.GroupNames(gen) {
			if keepSet[syntax.Ident{Kind: entities.KindValue, Name: name}] {
				return []ast.Decl{gen}
			}
		}

		return nil
	}

	return filterValueSpecs(fset, source, gen, keepSet)
}

func filterSpecs(
	gen *ast.GenDecl,
	keepSet map[syntax.Ident]bool,
	identOf func(ast.Spec) (syntax.Ident, bool),
) []ast.Decl {
	var specs []ast.Spec

	for _, spec := range gen.Specs {
		ident, ok := identOf(spec)
		if ok && keepSet[ident] {
			specs = append(specs, spec)
		}
	}

	return finishGenDecl(gen, specs)
}

func filterValueSpecs(
	fset *token.FileSet,
	source []byte,
	gen *ast.GenDecl,
	keepSet map[syntax.Ident]bool,
) []ast.Decl {
	var specs []ast.Spec

	for _, spec := range gen.Specs {
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

	return finishGenDecl(gen, specs)
}

func finishGenDecl(gen *ast.GenDecl, specs []ast.Spec) []ast.Decl {
	if len(specs) == 0 {
		return nil
	}

	if len(specs) < len(gen.Specs) {
		resetSpacingIfSafe(specs)
	}

	gen.Specs = specs

	return []ast.Decl{gen}
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
	switch typed := spec.(type) {
	case *ast.ValueSpec:
		for _, name := range typed.Names {
			name.NamePos = token.NoPos
		}
	case *ast.TypeSpec:
		typed.Name.NamePos = token.NoPos
	case *ast.ImportSpec:
		typed.Path.ValuePos = token.NoPos
		typed.EndPos = token.NoPos

		if typed.Name != nil {
			typed.Name.NamePos = token.NoPos
		}
	}
}
