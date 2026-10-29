package typed

import (
	"cmp"
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type Requirement struct {
	Dependent entities.DeclKey
	Owner     entities.DeclKey
	Methods   []entities.DeclKey
}

type Facts struct {
	// Forced are the methods a generic type parameter's constraint demands of
	// the type argument at an instantiation.
	Forced []Requirement
	// Converted are the methods of a concrete value that an interface it is
	// handed to demands.
	Converted []Requirement
	// Escaped are the methods of a concrete value that an interface it is
	// handed to can reach without any type naming them.
	Escaped []Requirement
	// Uses are the declarations each declaration names, whichever way: a call,
	// a type, a value, a method reached through a selector.
	Uses []Use
}

func (facts Facts) merged(other Facts) Facts {
	return Facts{
		Forced:    append(facts.Forced, other.Forced...),
		Converted: append(facts.Converted, other.Converted...),
		Escaped:   append(facts.Escaped, other.Escaped...),
		Uses:      append(facts.Uses, other.Uses...),
	}
}

type Use struct {
	Dependent, Provider entities.DeclKey
}

func collectFacts(pkgs []*packages.Package, root string, locator declLocator) Facts {
	scan := &factScan{where: newPositions(root, locator), dynamic: dynamicNames(pkgs)}

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		scan.pkg = pkg

		scan.instances()
		scan.uses()

		for _, file := range pkg.Syntax {
			ast.Walk(&conversionVisitor{scan: scan}, file)
		}
	}

	return scan.facts
}

func (facts Facts) sorted() Facts {
	slices.SortStableFunc(facts.Forced, compareRequirements)
	slices.SortStableFunc(facts.Converted, compareRequirements)
	slices.SortStableFunc(facts.Escaped, compareRequirements)

	slices.SortFunc(facts.Uses, func(left, right Use) int {
		return cmp.Or(
			cmp.Compare(left.Dependent.String(), right.Dependent.String()),
			cmp.Compare(left.Provider.String(), right.Provider.String()),
		)
	})

	facts.Uses = slices.Compact(facts.Uses)

	return facts
}

func compareRequirements(left, right Requirement) int {
	return cmp.Or(
		cmp.Compare(left.Dependent.String(), right.Dependent.String()),
		cmp.Compare(left.Owner.String(), right.Owner.String()),
	)
}

type factScan struct {
	where   positions
	pkg     *packages.Package
	facts   Facts
	dynamic map[string]bool
}

func (scan *factScan) instances() {
	checked := scan.pkg.TypesInfo

	for ident, instance := range checked.Instances {
		params := typeParamsOf(checked.Uses[ident])
		if params == nil {
			continue
		}

		dependent, inTree := scan.where.declAt(scan.pkg.Fset, ident.Pos())
		if !inTree {
			continue
		}

		for index := range min(params.Len(), instance.TypeArgs.Len()) {
			constraint, isInterface := params.At(index).Constraint().Underlying().(*types.Interface)
			if !isInterface {
				continue
			}

			scan.require(&scan.facts.Forced, dependent, instance.TypeArgs.At(index), constraint)
		}
	}
}

func typeParamsOf(generic types.Object) *types.TypeParamList {
	switch obj := generic.(type) {
	case *types.TypeName:
		if named, isNamed := obj.Type().(*types.Named); isNamed {
			return named.TypeParams()
		}
	case *types.Func:
		if sig, isSig := obj.Type().(*types.Signature); isSig {
			return sig.TypeParams()
		}
	}

	return nil
}
