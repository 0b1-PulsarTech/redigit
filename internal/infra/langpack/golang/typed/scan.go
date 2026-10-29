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

func (f Facts) merged(other Facts) Facts {
	return Facts{
		Forced:    append(f.Forced, other.Forced...),
		Converted: append(f.Converted, other.Converted...),
		Escaped:   append(f.Escaped, other.Escaped...),
		Uses:      append(f.Uses, other.Uses...),
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

func (f Facts) sorted() Facts {
	slices.SortStableFunc(f.Forced, compareRequirements)
	slices.SortStableFunc(f.Converted, compareRequirements)
	slices.SortStableFunc(f.Escaped, compareRequirements)

	slices.SortFunc(f.Uses, func(a, b Use) int {
		return cmp.Or(
			cmp.Compare(a.Dependent.String(), b.Dependent.String()),
			cmp.Compare(a.Provider.String(), b.Provider.String()),
		)
	})

	f.Uses = slices.Compact(f.Uses)

	return f
}

func compareRequirements(a, b Requirement) int {
	return cmp.Or(
		cmp.Compare(a.Dependent.String(), b.Dependent.String()),
		cmp.Compare(a.Owner.String(), b.Owner.String()),
	)
}

type factScan struct {
	where   positions
	pkg     *packages.Package
	facts   Facts
	dynamic map[string]bool
}

func (s *factScan) instances() {
	checked := s.pkg.TypesInfo

	for ident, instance := range checked.Instances {
		params := typeParamsOf(checked.Uses[ident])
		if params == nil {
			continue
		}

		dependent, inTree := s.where.declAt(s.pkg.Fset, ident.Pos())
		if !inTree {
			continue
		}

		for index := range min(params.Len(), instance.TypeArgs.Len()) {
			constraint, isInterface := params.At(index).Constraint().Underlying().(*types.Interface)
			if !isInterface {
				continue
			}

			s.require(&s.facts.Forced, dependent, instance.TypeArgs.At(index), constraint)
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
