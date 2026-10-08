package golang

import (
	"go/token"
	"go/types"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// require records the methods iface demands of value, as the declarations the
// tree holds for them. A value that is itself an interface, a type parameter or
// untyped demands nothing: only a concrete type owns methods to wait for.
func (s *factScan) require(
	into *[]requirement,
	dependent entities.DeclKey,
	value types.Type,
	iface *types.Interface,
) {
	if value == nil || iface.NumMethods() == 0 || !isConcrete(value) {
		return
	}

	var methods []entities.DeclKey

	for demanded := range iface.Methods() {
		found, _, _ := types.LookupFieldOrMethod(value, true, demanded.Pkg(), demanded.Name())

		fn, isFunc := found.(*types.Func)
		if !isFunc {
			continue
		}

		if key, inTree := s.where.declAt(s.pkg.Fset, fn.Origin().Pos()); inTree {
			methods = append(methods, key)
		}
	}

	if len(methods) == 0 {
		return
	}

	*into = append(*into, requirement{
		Dependent: dependent,
		Owner:     s.ownerOf(value),
		Methods:   methods,
	})
}

// ownerOf is the declaration of the named type behind value, zero for a type
// the tree does not declare.
func (s *factScan) ownerOf(value types.Type) entities.DeclKey {
	if pointer, isPointer := value.(*types.Pointer); isPointer {
		value = pointer.Elem()
	}

	named, isNamed := value.(*types.Named)
	if !isNamed {
		return entities.DeclKey{}
	}

	key, _ := s.where.declAt(s.pkg.Fset, named.Origin().Obj().Pos())

	return key
}

// isConcrete reports whether a type owns a method set of its own.
func isConcrete(value types.Type) bool {
	if _, isParam := value.(*types.TypeParam); isParam {
		return false
	}

	if basic, isBasic := value.(*types.Basic); isBasic && basic.Info()&types.IsUntyped != 0 {
		return false
	}

	return !types.IsInterface(value)
}

// convert records a value handed to a place that expects target, when target
// is an interface: the value must then have the methods target lists.
func (s *factScan) convert(at token.Pos, value, target types.Type) {
	if value == nil || target == nil {
		return
	}

	if _, isParam := target.(*types.TypeParam); isParam {
		return
	}

	iface, isInterface := target.Underlying().(*types.Interface)
	if !isInterface {
		return
	}

	dependent, inTree := s.where.declAt(s.pkg.Fset, at)
	if !inTree {
		return
	}

	s.require(&s.facts.Converted, dependent, value, iface)
	s.escape(dependent, value)
}
