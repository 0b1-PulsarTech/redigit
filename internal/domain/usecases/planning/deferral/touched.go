package deferral

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

type touchLinks struct {
	subjects map[entities.DeclKey][]entities.DeclKey
	atoms    map[string][]entities.DeclKey
}

func touched(
	graph entities.Graph,
	links touchLinks,
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

			needed := needsFresh(graph, links.subjects, key, fresh, out) ||
				mateTouched(graph, links.atoms, key, out)
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
		// A manifest line does not count: every test of a module needs its go.mod,
		// and that is no reason for the test to arrive with the first declaration
		// that pulled the manifest in.
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

func mateTouched(
	graph entities.Graph,
	atoms map[string][]entities.DeclKey,
	key entities.DeclKey,
	touched map[entities.DeclKey]bool,
) bool {
	return slices.ContainsFunc(atomMembers(graph, atoms, key), func(mate entities.DeclKey) bool {
		return touched[mate]
	})
}
