package slicing

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

type stubImporter struct{}

func (stubImporter) Import(path string) (*types.Package, error) {
	pkg := types.NewPackage(path, path[strings.LastIndex(path, "/")+1:])
	pkg.MarkComplete()

	return pkg, nil
}

type localTypes struct {
	*types.Info

	pkg      *types.Package
	selected map[*ast.Ident]bool
}

func typeLocals(fset *token.FileSet, file *ast.File) localTypes {
	info := &types.Info{
		Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{},
	}
	conf := types.Config{
		Importer:    stubImporter{},
		Error:       func(error) {},
		FakeImportC: true,
	}

	// The error is the point: the stubs make the check fail, and the objects are
	// recorded anyway.
	pkg, _ := conf.Check(file.Name.Name, fset, []*ast.File{file}, info)

	selected := map[*ast.Ident]bool{}

	ast.Inspect(file, func(node ast.Node) bool {
		if selector, isSelector := node.(*ast.SelectorExpr); isSelector {
			selected[selector.Sel] = true
		}

		return true
	})

	return localTypes{Info: info, pkg: pkg, selected: selected}
}

func (found localTypes) objectOf(ident *ast.Ident) types.Object {
	if obj := found.Defs[ident]; obj != nil {
		return obj
	}

	if obj := found.Uses[ident]; obj != nil {
		return obj
	}

	// The check records no use for a name it could not place, such as the key of a
	// literal of an imported type, so the scope at the name is asked instead. The
	// scope at a name is the one that holds the name an inner declaration shadows.
	scope := found.pkg.Scope().Innermost(ident.Pos())
	if scope == nil || found.selected[ident] {
		return nil
	}

	_, obj := scope.LookupParent(ident.Name, ident.Pos())

	return obj
}
