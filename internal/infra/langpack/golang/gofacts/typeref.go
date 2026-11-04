package gofacts

import (
	"go/token"
	"go/types"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type refs struct {
	where typed.Positions
	fset  *token.FileSet
}

func (scope refs) ref(typ types.Type) facts.TypeRef {
	return facts.TypeRef{Key: scope.keyOf(typ), Name: types.TypeString(unaliased(typ), qualify)}
}

func unaliased(typ types.Type) types.Type {
	switch cur := types.Unalias(typ).(type) {
	case *types.Pointer:
		return types.NewPointer(unaliased(cur.Elem()))
	case *types.Slice:
		return types.NewSlice(unaliased(cur.Elem()))
	case *types.Array:
		return types.NewArray(unaliased(cur.Elem()), cur.Len())
	case *types.Chan:
		return types.NewChan(cur.Dir(), unaliased(cur.Elem()))
	case *types.Map:
		return types.NewMap(unaliased(cur.Key()), unaliased(cur.Elem()))
	case *types.Signature:
		return types.NewSignatureType(
			cur.Recv(), nil, nil,
			unaliasedTuple(cur.Params()), unaliasedTuple(cur.Results()), cur.Variadic(),
		)
	case *types.Named:
		return unaliasedNamed(cur)
	default:
		return cur
	}
}

func unaliasedTuple(tuple *types.Tuple) *types.Tuple {
	vars := make([]*types.Var, tuple.Len())
	for idx := range vars {
		field := tuple.At(idx)
		vars[idx] = types.NewVar(field.Pos(), field.Pkg(), field.Name(), unaliased(field.Type()))
	}

	return types.NewTuple(vars...)
}

func unaliasedNamed(named *types.Named) types.Type {
	args := named.TypeArgs()
	if args.Len() == 0 {
		return named
	}

	list := make([]types.Type, args.Len())
	for idx := range list {
		list[idx] = unaliased(args.At(idx))
	}

	instance, err := types.Instantiate(nil, named.Origin(), list, false)
	if err != nil {
		return named
	}

	return instance
}

func (scope refs) receiver(name *types.TypeName) facts.TypeRef {
	return facts.TypeRef{Key: scope.declOf(name), Name: qualified(name)}
}

func (scope refs) declOf(name *types.TypeName) entities.DeclKey {
	// Only a package-scope type is a declaration; DeclAt keys a local one as its enclosing func.
	if name.Pkg() == nil || name.Parent() != name.Pkg().Scope() {
		return entities.DeclKey{}
	}

	key, _ := scope.where.ObjectAt(scope.fset, name)

	return key
}

func (scope refs) keyOf(typ types.Type) entities.DeclKey {
	// The declaration a type stands for is the named type under its pointer, slice, array or channel.
	for {
		switch cur := types.Unalias(typ).(type) {
		case *types.Pointer:
			typ = cur.Elem()
		case *types.Slice:
			typ = cur.Elem()
		case *types.Array:
			typ = cur.Elem()
		case *types.Chan:
			typ = cur.Elem()
		case *types.Named:
			return scope.declOf(cur.Origin().Obj())
		default:
			return entities.DeclKey{}
		}
	}
}

func qualify(pkg *types.Package) string {
	return pkg.Path()
}

func qualified(name *types.TypeName) string {
	if name.Pkg() == nil {
		return name.Name()
	}

	return name.Pkg().Path() + "." + name.Name()
}
