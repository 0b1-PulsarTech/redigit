package modmap

import (
	"go/token"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

func Seed(root string, pkgs []*packages.Package, names map[string]string) {
	files := New(root).Reading(pkgs)
	seen := map[*token.FileSet]bool{}

	for _, pkg := range pkgs {
		if pkg.Fset == nil || seen[pkg.Fset] {
			continue
		}

		seen[pkg.Fset] = true

		pkg.Fset.Iterate(func(file *token.File) bool {
			name := file.Name()
			if filepath.IsAbs(name) {
				return true
			}

			rel := files.Rel(name)
			// Another group read the same name against its own modules; a disagreement maps nothing.
			if known := names[name]; known != "" && known != rel {
				rel = ""
			}

			names[name] = rel

			return true
		})
	}
}
