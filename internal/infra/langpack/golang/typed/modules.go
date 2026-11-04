package typed

import (
	"fmt"
	"go/ast"
	"path"
	"slices"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type module struct {
	dir  string
	path string
}

func modulesOf(snap entities.Snapshot) ([]module, error) {
	var modules []module

	for _, file := range snap.Files {
		if path.Base(file) != "go.mod" {
			continue
		}

		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return nil, fmt.Errorf("go.mod source: %w", err)
		}

		if name := modfile.ModulePath(source); name != "" {
			modules = append(modules, module{dir: path.Dir(file), path: name})
		}
	}

	return modules, nil
}

func allPatterns(modules []module) []string {
	patterns := make([]string, 0, len(modules))
	for _, found := range modules {
		patterns = append(patterns, found.path+"/...")
	}

	slices.Sort(patterns)

	return patterns
}

var protocolMethods = map[string]bool{
	"Error": true, "Unwrap": true, "Is": true, "As": true,
	"String": true, "GoString": true, "Format": true,
	"MarshalJSON": true, "UnmarshalJSON": true,
	"MarshalText": true, "UnmarshalText": true,
	"Scan": true, "Value": true,
}

func dynamicNames(pkgs []*packages.Package) map[string]bool {
	// Dynamic methods stay reachable through an interface: stdlib protocols plus the tree's own.
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
