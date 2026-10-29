package planning

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// touched keeps the ready declarations that need something fresh, directly or
// through another ready declaration. A manifest line does not count: every test
// of a module needs its go.mod, and that is no reason for the test to arrive
// with the first declaration that pulled the manifest in.
func touched(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	own owners.Ownership,
	ready, fresh map[entities.DeclKey]bool,
) map[entities.DeclKey]bool {
	out := map[entities.DeclKey]bool{}

	for changed := true; changed; {
		changed = false

		for key := range ready {
			if out[key] {
				continue
			}

			needed := needsFresh(graph, subjects, key, fresh, out)
			if own.Loose[key] {
				needed = needed || reach.TiedToFresh(graph, key, fresh, out)
			}

			if !needed && reach.IsAfterwordBase(graph, key) {
				needed = consumedBy(graph, key, out)
			}

			if !needed {
				continue
			}

			out[key] = true
			changed = true
		}
	}

	return out
}

func needsFresh(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	key entities.DeclKey,
	fresh, touched map[entities.DeclKey]bool,
) bool {
	for _, edge := range graph.Needs(key) {
		if entities.IsStructural(edge.Reason) || edge.Reason == entities.ReasonImport {
			continue
		}

		if fresh[edge.Provider] || touched[edge.Provider] {
			return true
		}
	}

	for _, subject := range subjects[key] {
		if fresh[subject] || touched[subject] {
			return true
		}
	}

	return false
}

func consumedBy(
	graph entities.Graph,
	key entities.DeclKey,
	touched map[entities.DeclKey]bool,
) bool {
	for _, edge := range graph.Consumers(key) {
		if touched[edge.Dependent] {
			return true
		}
	}

	return false
}
