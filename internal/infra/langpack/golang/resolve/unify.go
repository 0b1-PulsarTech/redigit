package resolve

import (
	"go/token"
	"go/types"
)

const maxUnifyDepth = 64

type unifier struct {
	bound map[*types.TypeParam]types.Type
	fset  *token.FileSet
	depth int
}

func newUnifier(fset *token.FileSet) *unifier {
	return &unifier{bound: map[*types.TypeParam]types.Type{}, fset: fset}
}

func (uni *unifier) resolve(typ types.Type) types.Type {
	for {
		param, isParam := types.Unalias(typ).(*types.TypeParam)
		if !isParam {
			return types.Unalias(typ)
		}

		next, seen := uni.bound[param]
		if !seen {
			return param
		}

		typ = next
	}
}

func (uni *unifier) bind(param *types.TypeParam, typ types.Type) bool {
	// A parameter bound to a type that holds it never settles, so it matches nothing.
	if uni.mentions(param, typ, 0) {
		return false
	}

	uni.bound[param] = typ

	return true
}

func (uni *unifier) match(left, right types.Type) bool {
	// Identity comes first: resolving a parameter bound to a type that holds itself would not end.
	if left == right {
		return true
	}

	if uni.depth >= maxUnifyDepth {
		return false
	}

	uni.depth++
	defer func() { uni.depth-- }()

	left, right = uni.resolve(left), uni.resolve(right)
	if left == right {
		return true
	}

	// An unbound parameter on either side takes the other type, and keeps it for every later use.
	if param, isParam := left.(*types.TypeParam); isParam {
		return uni.bind(param, right)
	}

	if param, isParam := right.(*types.TypeParam); isParam {
		return uni.bind(param, left)
	}

	switch left := left.(type) {
	case *types.Named:
		return uni.named(left, right)
	case *types.Signature:
		other, same := right.(*types.Signature)

		return same && left.Variadic() == other.Variadic() &&
			uni.tuple(left.Params(), other.Params()) &&
			uni.tuple(left.Results(), other.Results())
	case *types.Interface:
		return uni.iface(left, right)
	}

	return uni.element(left, right)
}

func (uni *unifier) element(left, right types.Type) bool {
	switch left := left.(type) {
	case *types.Pointer:
		other, same := right.(*types.Pointer)

		return same && uni.match(left.Elem(), other.Elem())
	case *types.Slice:
		other, same := right.(*types.Slice)

		return same && uni.match(left.Elem(), other.Elem())
	case *types.Array:
		other, same := right.(*types.Array)

		return same && left.Len() == other.Len() && uni.match(left.Elem(), other.Elem())
	case *types.Chan:
		other, same := right.(*types.Chan)

		return same && left.Dir() == other.Dir() && uni.match(left.Elem(), other.Elem())
	case *types.Map:
		other, same := right.(*types.Map)

		return same && uni.match(left.Key(), other.Key()) &&
			uni.match(left.Elem(), other.Elem())
	case *types.Struct:
		return uni.structure(left, right)
	}

	return types.Identical(left, right)
}

func (uni *unifier) named(left *types.Named, right types.Type) bool {
	other, same := right.(*types.Named)
	if !same || !uni.sameDeclaration(left.Origin().Obj(), other.Origin().Obj()) ||
		left.TypeArgs().Len() != other.TypeArgs().Len() {
		return false
	}

	for idx := range left.TypeArgs().Len() {
		if !uni.match(left.TypeArgs().At(idx), other.TypeArgs().At(idx)) {
			return false
		}
	}

	return true
}

func (uni *unifier) iface(left *types.Interface, right types.Type) bool {
	other, same := right.(*types.Interface)
	if !same || left.NumMethods() != other.NumMethods() {
		return false
	}

	// Type-set terms are not unified, so an interface holding any is compared as written.
	if !left.IsMethodSet() || !other.IsMethodSet() {
		return types.Identical(left, other)
	}

	for want := range left.Methods() {
		matched := false

		for have := range other.Methods() {
			if have.Id() == want.Id() {
				matched = uni.match(want.Type(), have.Type())

				break
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

func (uni *unifier) tuple(left, right *types.Tuple) bool {
	if left.Len() != right.Len() {
		return false
	}

	for idx := range left.Len() {
		if !uni.match(left.At(idx).Type(), right.At(idx).Type()) {
			return false
		}
	}

	return true
}
