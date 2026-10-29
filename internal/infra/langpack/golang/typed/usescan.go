package typed

import (
	"go/types"
)

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
