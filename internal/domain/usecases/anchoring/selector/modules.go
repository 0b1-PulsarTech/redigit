package selector

import (
	"path"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func ModuleDirs(graph entities.Graph) map[string]bool {
	dirs := map[string]bool{}

	for _, edge := range graph.Edges() {
		if edge.Reason == entities.ReasonModule {
			dirs[path.Dir(edge.Provider.File)] = true
		}
	}

	return dirs
}
