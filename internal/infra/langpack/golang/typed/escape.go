package typed

import (
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var protocolMethods = map[string]bool{
	"Error": true, "Unwrap": true, "Is": true, "As": true,
	"String": true, "GoString": true, "Format": true,
	"MarshalJSON": true, "UnmarshalJSON": true,
	"MarshalText": true, "UnmarshalText": true,
	"Scan": true, "Value": true,
}

func dynamicNames(pkgs []*packages.Package) map[string]bool {
	names := map[string]bool{}

	for name := range protocolMethods {
		names[name] = true
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				iface, isInterface := node.(*ast.InterfaceType)
				if !isInterface {
					return true
				}

				for _, field := range iface.Methods.List {
					for _, name := range field.Names {
						names[name.Name] = true
					}
				}

				return true
			})
		}
	}

	return names
}

func (s *factScan) escape(dependent entities.DeclKey, value types.Type) {
	if value == nil || !isConcrete(value) {
		return
	}

	owner := s.ownerOf(value)

	if owner != (entities.DeclKey{}) && belongsTo(dependent, owner) {
		return
	}

	var methods []entities.DeclKey

	set := types.NewMethodSet(value)

	for selection := range set.Methods() {
		fn, isFunc := selection.Obj().(*types.Func)
		if !isFunc || !s.dynamic[fn.Name()] {
			continue
		}

		if key, inTree := s.where.declAt(s.pkg.Fset, fn.Origin().Pos()); inTree {
			methods = append(methods, key)
		}
	}

	if len(methods) == 0 {
		return
	}

	s.facts.Escaped = append(s.facts.Escaped, Requirement{
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

func (s *factScan) require(
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

		if key, inTree := s.where.declAt(s.pkg.Fset, fn.Origin().Pos()); inTree {
			methods = append(methods, key)
		}
	}

	if len(methods) == 0 {
		return
	}

	*into = append(*into, Requirement{
		Dependent: dependent,
		Owner:     s.ownerOf(value),
		Methods:   methods,
	})
}

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

func isConcrete(value types.Type) bool {
	if _, isParam := value.(*types.TypeParam); isParam {
		return false
	}

	if basic, isBasic := value.(*types.Basic); isBasic && basic.Info()&types.IsUntyped != 0 {
		return false
	}

	return !types.IsInterface(value)
}

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
