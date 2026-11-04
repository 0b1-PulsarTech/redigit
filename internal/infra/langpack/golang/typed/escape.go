package typed

import (
	"go/token"
	"go/types"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (scan *factScan) escape(dependent entities.DeclKey, value types.Type) {
	if value == nil || !isConcrete(value) {
		return
	}

	value = types.Unalias(value)
	owner := scan.ownerOf(value)

	if owner != (entities.DeclKey{}) && pointee(value) == value && belongsTo(dependent, owner) {
		return
	}

	var methods []entities.DeclKey

	set := types.NewMethodSet(pointee(value))

	for selection := range set.Methods() {
		fn, isFunc := selection.Obj().(*types.Func)
		if !isFunc || !scan.dynamic[fn.Name()] {
			continue
		}

		key, inTree := scan.where.ObjectAt(scan.pkg.Fset, fn.Origin())
		if inTree && key != dependent {
			methods = append(methods, key)
		}
	}

	if len(methods) == 0 {
		return
	}

	scan.facts.Escaped = append(scan.facts.Escaped, Requirement{
		Dependent: dependent,
		Owner:     owner,
		Methods:   methods,
	})
}

func belongsTo(dependent, owner entities.DeclKey) bool {
	if dependent == owner {
		return true
	}

	if dependent.Kind != entities.KindMethod || path.Dir(dependent.File) != path.Dir(owner.File) {
		return false
	}

	receiver, _, _ := strings.Cut(dependent.Name, ".")

	return strings.TrimPrefix(receiver, "*") == owner.Name
}

func (scan *factScan) require(
	into *[]Requirement,
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

		if key, inTree := scan.where.ObjectAt(scan.pkg.Fset, fn.Origin()); inTree {
			methods = append(methods, key)
		}
	}

	if len(methods) == 0 {
		return
	}

	*into = append(*into, Requirement{
		Dependent: dependent,
		Owner:     scan.ownerOf(value),
		Methods:   methods,
	})
}

func (scan *factScan) ownerOf(value types.Type) entities.DeclKey {
	for {
		pointer, isPointer := types.Unalias(value).(*types.Pointer)
		if !isPointer {
			break
		}

		value = types.Unalias(pointer.Elem())
	}

	named, isNamed := types.Unalias(value).(*types.Named)
	if !isNamed {
		return entities.DeclKey{}
	}

	key, _ := scan.where.ObjectAt(scan.pkg.Fset, named.Origin().Obj())

	return key
}

func isConcrete(value types.Type) bool {
	if _, isParam := value.(*types.TypeParam); isParam {
		return false
	}

	if basic, isBasic := value.(*types.Basic); isBasic && basic.Info()&types.IsUntyped != 0 {
		return false
	}

	return !types.IsInterface(value)
}

func (scan *factScan) demand(at token.Pos, value, target types.Type) (entities.DeclKey, bool) {
	if _, isParam := target.(*types.TypeParam); target == nil || isParam {
		return entities.DeclKey{}, false
	}

	iface, isInterface := target.Underlying().(*types.Interface)

	dependent, inTree := scan.where.DeclAt(scan.pkg.Fset, at)
	if !isInterface || !inTree {
		return entities.DeclKey{}, false
	}

	scan.require(&scan.facts.Converted, dependent, value, iface)

	return dependent, true
}

func (scan *factScan) convert(at token.Pos, value, target types.Type) {
	if dependent, isDemanded := scan.demand(at, value, target); isDemanded {
		scan.escape(dependent, value)
	}
}

func pointee(value types.Type) types.Type {
	// A pointer to a pointer has no methods of its own; what it can reach are its pointee's.
	for {
		outer, isPointer := types.Unalias(value).(*types.Pointer)
		if !isPointer {
			return value
		}

		if _, isNested := types.Unalias(outer.Elem()).(*types.Pointer); !isNested {
			return value
		}

		value = types.Unalias(outer.Elem())
	}
}
