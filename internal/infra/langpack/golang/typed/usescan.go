package typed

import (
	"go/types"
)

func (scan *factScan) uses() {
	for ident, obj := range scan.pkg.TypesInfo.Uses {
		if obj.Pkg() == nil {
			continue
		}

		if field, isVar := obj.(*types.Var); isVar && field.IsField() {
			continue
		}

		if fn, isFunc := obj.(*types.Func); isFunc {
			obj = fn.Origin()
		}

		dependent, inTree := scan.where.declAt(scan.pkg.Fset, ident.Pos())
		if !inTree {
			continue
		}

		provider, found := scan.where.declAt(scan.pkg.Fset, obj.Pos())
		if !found || provider == dependent {
			continue
		}

		scan.facts.Uses = append(scan.facts.Uses, Use{Dependent: dependent, Provider: provider})
	}
}
