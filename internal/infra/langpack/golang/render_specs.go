package golang

import (
	"go/ast"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func filterGenDecl(
	fset *token.FileSet,
	source []byte,
	d *ast.GenDecl,
	keepSet map[declIdent]bool,
) []ast.Decl {
	if d.Tok == token.TYPE {
		return filterSpecs(d, keepSet, func(spec ast.Spec) (declIdent, bool) {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				return declIdent{}, false
			}

			return declIdent{entities.KindType, ts.Name.Name}, true
		})
	}

	if d.Tok == token.CONST && hasImplicitSpec(d) {
		// Any surviving name keeps the whole group: an iota constant that is
		// not written down still takes its value from its position in the
		// block, so the block cannot be cut.
		for _, name := range groupNames(d) {
			if keepSet[declIdent{entities.KindValue, name}] {
				return []ast.Decl{d}
			}
		}

		return nil
	}

	return filterValueSpecs(fset, source, d, keepSet)
}

// filterSpecs keeps the specs whose identifier (from identOf) is in keepSet,
// dropping the whole GenDecl when none survive.
func filterSpecs(
	d *ast.GenDecl,
	keepSet map[declIdent]bool,
	identOf func(ast.Spec) (declIdent, bool),
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

// filterValueSpecs keeps a const/var spec when any of its bound names is
// kept. A spec binding several names (var a, b = f()) survives as a whole:
// Go cannot drop one name out of a shared initializer. A blank identifier is
// looked up the same disambiguated way Declarations built its key, or every
// blank spec in the group would match the first keepSet entry.
func filterValueSpecs(
	fset *token.FileSet,
	source []byte,
	d *ast.GenDecl,
	keepSet map[declIdent]bool,
) []ast.Decl {
	var specs []ast.Spec

	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, name := range vs.Names {
			if keepSet[declIdent{entities.KindValue, valueName(fset, source, vs, name)}] {
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

// resetSpacingIfSafe clears the spacing a dropped sibling would otherwise
// leave as a stray blank line, by detaching each surviving spec's own
// position from the source it was parsed at. It is skipped whenever any spec
// in the group carries its own doc comment: go/printer places a *ast.File's
// comments by comparing their source position against the surrounding code,
// and a spec repositioned to NoPos while its comment keeps its real position
// inverts that order. Left alone, the stray blank line is still valid,
// gofmt-idempotent output - gofmt preserves an existing blank line rather
// than judging it accidental.
func resetSpacingIfSafe(specs []ast.Spec) {
	for _, spec := range specs {
		if specDoc(spec) != nil {
			return
		}
	}

	for _, spec := range specs {
		resetSpecPos(spec)
	}
}

func specDoc(spec ast.Spec) *ast.CommentGroup {
	switch s := spec.(type) {
	case *ast.ValueSpec:
		return s.Doc
	case *ast.TypeSpec:
		return s.Doc
	case *ast.ImportSpec:
		return s.Doc
	default:
		return nil
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
