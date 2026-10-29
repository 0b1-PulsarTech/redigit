package sequence

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func FeatureNeeds(
	graph entities.Graph,
	own owners.Ownership,
) map[reach.FeatureID]map[reach.FeatureID]bool {
	needs := map[reach.FeatureID]map[reach.FeatureID]bool{}
	behind := map[entities.DeclKey]map[reach.FeatureID]bool{}

	add := func(from, to reach.FeatureID) {
		if from == reach.SharedFeature || to == reach.SharedFeature || from == to {
			return
		}

		if needs[from] == nil {
			needs[from] = map[reach.FeatureID]bool{}
		}

		needs[from][to] = true
	}

	for _, edge := range graph.Edges() {
		from := own.Owner[edge.Dependent]
		add(from, own.Owner[edge.Provider])

		// What a deferred declaration needs is not shared by construction: it
		// was shared by being deferred. A feature reaching it is built after
		// the features it waits for.
		if own.Deferred[edge.Provider] && from != reach.SharedFeature {
			for feature := range featuresBehind(graph, own, edge.Provider, behind) {
				add(from, feature)
			}
		}
	}

	return needs
}

func featuresBehind(
	graph entities.Graph,
	own owners.Ownership,
	start entities.DeclKey,
	memo map[entities.DeclKey]map[reach.FeatureID]bool,
) map[reach.FeatureID]bool {
	if found, ok := memo[start]; ok {
		return found
	}

	out := map[reach.FeatureID]bool{}
	seen := map[entities.DeclKey]bool{start: true}
	queue := []entities.DeclKey{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range graph.Needs(current) {
			if seen[edge.Provider] {
				continue
			}

			seen[edge.Provider] = true

			if owner := own.Owner[edge.Provider]; owner != reach.SharedFeature {
				out[owner] = true

				continue
			}

			if own.Deferred[edge.Provider] {
				queue = append(queue, edge.Provider)
			}
		}
	}

	memo[start] = out

	return out
}

func Earliest(own owners.Ownership, order entities.Ordering) map[reach.FeatureID]entities.Position {
	out := map[reach.FeatureID]entities.Position{}
	// The earliest position is the tie-break between features neither of which
	// needs the other, so a rebuilt history still follows the original
	// chronology wherever the dependency graph leaves the choice open.

	for key, feature := range own.Owner {
		at := order.Of(key)
		if !at.IsSet() {
			continue
		}

		if current, found := out[feature]; !found || at < current {
			out[feature] = at
		}
	}

	return out
}
