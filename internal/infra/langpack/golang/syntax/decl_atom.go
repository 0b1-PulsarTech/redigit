package syntax

import (
	"go/ast"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func NeedsItsBlock(decl *ast.GenDecl) bool {
	for index, spec := range decl.Specs {
		if index == 0 {
			continue
		}

		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		// An explicit spec that reads iota takes its value from its position, so it needs its block too.
		if (vs.Type == nil && len(vs.Values) == 0) || usesIota(vs) {
			return true
		}
	}

	return false
}

func usesIota(vs *ast.ValueSpec) bool {
	found := false

	for _, value := range vs.Values {
		ast.Inspect(value, func(node ast.Node) bool {
			ident, isIdent := node.(*ast.Ident)
			if isIdent && ident.Name == "iota" {
				found = true
			}

			return !found
		})
	}

	return found
}

func atomLabel(prefix string, fset *token.FileSet, pos token.Pos) string {
	// Not Position: a //line directive would give two blocks of one file the same label.
	return prefix + ":" + fset.PositionFor(pos, false).String()
}

func buildAtomicGroup(
	fset *token.FileSet,
	path string,
	source []byte,
	unit entities.UnitID,
	decl *ast.GenDecl,
) []entities.Decl {
	start := decl.Pos()
	if decl.Doc != nil {
		start = decl.Doc.Pos()
	}

	// Every name shares the group's span, so whichever one a plan names, the
	// renderer emits the whole group and the constants keep their values.
	span := spanOf(fset, start, decl.End())

	size := len(GroupNames(decl))
	decls := make([]entities.Decl, 0, size)

	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, name := range vs.Names {
			decls = append(decls, entities.Decl{
				Key:       entities.DeclKey{File: path, Kind: entities.KindValue, Name: name.Name},
				Unit:      unit,
				Span:      span,
				Atom:      atomLabel("const-group", fset, decl.Pos()),
				Selector:  LocationOf(fset, path, source, name.Pos()),
				GroupSize: size,
			})
		}
	}

	return decls
}

func GroupNames(decl *ast.GenDecl) []string {
	var names []string

	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, name := range vs.Names {
			names = append(names, name.Name)
		}
	}

	return names
}
