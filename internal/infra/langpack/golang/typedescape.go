package golang

import (
	"go/ast"
	"go/types"
	"path"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// protocolMethods are the methods the standard library calls on a value it
// holds as an interface or any: errors.Is/As/Unwrap, fmt's verbs, encoding
// packages and the sort and flag hooks. No call in the tree names them.
var protocolMethods = map[string]bool{
	"Error": true, "Unwrap": true, "Is": true, "As": true,
	"String": true, "GoString": true, "Format": true,
	"MarshalJSON": true, "UnmarshalJSON": true,
	"MarshalText": true, "UnmarshalText": true,
	"Scan": true, "Value": true,
}

// dynamicNames is every method name a value can be reached by once it sits in
// an interface: the standard library's protocols plus each method the tree's
// own interfaces declare, which an errors.As or a type assertion to a
// duck-typed port (StatusCode) asks of whatever the value is.
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

// escape records the dynamic methods of a concrete value handed to an
// interface, whatever that interface lists. Nothing static reaches them from
// the declaration that hands the value over: "fmt.Errorf format %w has arg
// problem.MissingBody of wrong type" failed every tree between the commit that
// wrapped the value and the one that gave the type its Error method. A method
// of the dependent's own type is left out: siblings of one type do not wait
// for each other.
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

	s.facts.Escaped = append(s.facts.Escaped, requirement{
		Dependent: dependent,
		Owner:     owner,
		Methods:   methods,
	})
}

// belongsTo reports whether dependent is the type itself or one of its methods
// (a method's name is "Type.Method", with a "*" before the type when the
// receiver is a pointer).
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
