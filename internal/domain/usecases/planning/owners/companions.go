package owners

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func shareCompanions(graph entities.Graph, own Ownership) {
	for _, decl := range graph.Decls() {
		if decl.Key.Kind != entities.KindFunc || own.Owner[decl.Key] == reach.SharedFeature ||
			entities.IsTestSide(decl) {
			continue
		}

		for _, edge := range graph.Needs(decl.Key) {
			provider, found := graph.Decl(edge.Provider)
			if found && provider.Key.Kind == entities.KindType && provider.Unit == decl.Unit &&
				own.Owner[provider.Key] == reach.SharedFeature && !own.Deferred[provider.Key] &&
				!entities.IsStructural(edge.Reason) {
				own.Owner[decl.Key] = reach.SharedFeature

				break
			}
		}
	}
}
