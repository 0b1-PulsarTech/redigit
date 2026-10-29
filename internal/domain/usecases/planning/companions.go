package planning

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// shareCompanions makes a function that builds or reads a shared type, in the
// package of that type, shared with it. A constructor is read only by the code
// that registers it, so a feature owned it alone and the type stood in the
// shared kernel without it: every test of the type waited for that feature.
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
