package typed

import (
	"fmt"
	"path"
	"slices"

	"golang.org/x/mod/modfile"

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
