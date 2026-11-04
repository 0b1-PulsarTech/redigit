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
	return facts.TypeRef{Key: scope.keyOf(typ), Name: types.TypeString(typ, qualify)}
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
