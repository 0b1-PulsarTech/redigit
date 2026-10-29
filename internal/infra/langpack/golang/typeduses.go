package golang

import (
	"go/types"
)

// uses reads every identifier of the package that names a declaration of the
// tree, from the checker's own resolution of it. A reference query answers the
// same question per declaration and can refuse; the checker has already
// answered it for every identifier. Fields are no declaration of their own (the
// type that holds one is), and a name that resolves inside its own declaration
// orders nothing.
func (s *factScan) uses() {
	for ident, obj := range s.pkg.TypesInfo.Uses {
		if obj.Pkg() == nil {
			continue
		}

		if field, isVar := obj.(*types.Var); isVar && field.IsField() {
			continue
		}

		if fn, isFunc := obj.(*types.Func); isFunc {
			obj = fn.Origin()
		}

		dependent, inTree := s.where.declAt(s.pkg.Fset, ident.Pos())
		if !inTree {
			continue
		}

		provider, found := s.where.declAt(s.pkg.Fset, obj.Pos())
		if !found || provider == dependent {
			continue
		}

		s.facts.Uses = append(s.facts.Uses, Use{Dependent: dependent, Provider: provider})
	}
}
