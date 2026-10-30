package resolve

import (
	"go/types"
	"slices"
)

func (uni *unifier) mentions(param *types.TypeParam, typ types.Type, depth int) bool {
	// A type too deep to walk is refused rather than guessed at.
	if depth >= maxUnifyDepth {
		return true
	}

	typ = uni.resolve(typ)
	if found, isParam := typ.(*types.TypeParam); isParam {
		return found == param
	}

	for _, part := range parts(typ) {
		if uni.mentions(param, part, depth+1) {
			return true
		}
	}

	return false
}

func (uni *unifier) sameDeclaration(left, right *types.TypeName) bool {
	if left == right {
		return true
	}

	// A package recompiled for a test has its own objects for the same source, so the
	// declaration site stands in for the object; gopls reads a type that way too.
	if uni.fset == nil || left.Pkg() == nil || right.Pkg() == nil ||
		left.Name() != right.Name() || left.Pkg().Path() != right.Pkg().Path() ||
		!left.Pos().IsValid() || !right.Pos().IsValid() {
		return false
	}

	from, to := uni.fset.PositionFor(left.Pos(), false), uni.fset.PositionFor(right.Pos(), false)

	return from.Filename == to.Filename && from.Offset == to.Offset
}

func (uni *unifier) structure(left *types.Struct, right types.Type) bool {
	other, same := right.(*types.Struct)
	if !same || left.NumFields() != other.NumFields() {
		return false
	}

	for idx := range left.NumFields() {
		want, have := left.Field(idx), other.Field(idx)

		if want.Id() != have.Id() || want.Embedded() != have.Embedded() ||
			left.Tag(idx) != other.Tag(idx) || !uni.match(want.Type(), have.Type()) {
			return false
		}
	}

	return true
}

func parts(typ types.Type) []types.Type {
	switch typ := typ.(type) {
	case *types.Named:
		return slices.Collect(typ.TypeArgs().Types())
	case *types.Pointer:
		return []types.Type{typ.Elem()}
	case *types.Slice:
		return []types.Type{typ.Elem()}
	case *types.Array:
		return []types.Type{typ.Elem()}
	case *types.Chan:
		return []types.Type{typ.Elem()}
	case *types.Map:
		return []types.Type{typ.Key(), typ.Elem()}
	case *types.Signature:
		return append(variables(typ.Params()), variables(typ.Results())...)
	case *types.Struct:
		var out []types.Type

		for field := range typ.Fields() {
			out = append(out, field.Type())
		}

		return out
	case *types.Interface:
		var out []types.Type

		for method := range typ.Methods() {
			out = append(out, method.Type())
		}

		return out
	}

	return nil
}

func variables(tuple *types.Tuple) []types.Type {
	var out []types.Type

	for variable := range tuple.Variables() {
		out = append(out, variable.Type())
	}

	return out
}
