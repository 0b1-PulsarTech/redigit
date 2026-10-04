package golang

import (
	"cmp"
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// requirement says that Dependent compiles only once Methods exist. Owner is
// the declaration of the type those methods hang off, so a rule can tell a type
// that needs its own methods from one that needs another type's.
type requirement struct {
	Dependent entities.DeclKey
	Owner     entities.DeclKey
	Methods   []entities.DeclKey
}

// typedFacts is what the type checker proved about the tree.
type typedFacts struct {
	// Forced are the methods a generic type parameter's constraint demands of
	// the type argument at an instantiation.
	Forced []requirement
	// Converted are the methods of a concrete value that an interface it is
	// handed to demands.
	Converted []requirement
}

func collectFacts(pkgs []*packages.Package, root string, locator declLocator) typedFacts {
	scan := &factScan{where: newPositions(root, locator)}

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		scan.pkg = pkg

		scan.instances()

		for _, file := range pkg.Syntax {
			ast.Walk(&conversionVisitor{scan: scan}, file)
		}
	}

	return scan.facts.sorted()
}

// sorted makes the facts independent of map iteration order: the checker keeps
// its instances in a map, and an order that changed between runs would change
// the plan between runs.
func (f typedFacts) sorted() typedFacts {
	slices.SortStableFunc(f.Forced, compareRequirements)
	slices.SortStableFunc(f.Converted, compareRequirements)

	return f
}

func compareRequirements(a, b requirement) int {
	return cmp.Or(
		cmp.Compare(a.Dependent.String(), b.Dependent.String()),
		cmp.Compare(a.Owner.String(), b.Owner.String()),
	)
}

// factScan is the working state of one collectFacts call.
type factScan struct {
	where positions
	pkg   *packages.Package
	facts typedFacts
}

// instances reads every generic instantiation of the package. A type argument
// must satisfy its parameter's constraint, and a constraint that names methods
// demands them of the argument: that is the check go makes, so the methods are
// asked of the checker rather than guessed from the argument's shape.
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

// typeParamsOf is the type parameter list of a generic type or function, nil
// for anything else.
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
