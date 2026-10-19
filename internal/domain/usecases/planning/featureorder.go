package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// featureNeeds is which features have to be built before which. A feature needs
// another when any declaration it owns waits on a declaration the other owns.
func featureNeeds(graph entities.Graph, own ownership) map[FeatureID]map[FeatureID]bool {
	needs := map[FeatureID]map[FeatureID]bool{}
	behind := map[entities.DeclKey]map[FeatureID]bool{}

	add := func(from, to FeatureID) {
		if from == SharedFeature || to == SharedFeature || from == to {
			return
		}

		if needs[from] == nil {
			needs[from] = map[FeatureID]bool{}
		}

		needs[from][to] = true
	}

	for _, edge := range graph.Edges() {
		from := own.owner[edge.Dependent]
		add(from, own.owner[edge.Provider])

		// What a deferred declaration needs is not shared by construction: it
		// was shared by being deferred. A feature reaching it is built after
		// the features it waits for.
		if own.deferred[edge.Provider] && from != SharedFeature {
			for feature := range featuresBehind(graph, own, edge.Provider, behind) {
				add(from, feature)
			}
		}
	}

	return needs
}

// featuresBehind is the features that own what a deferred declaration needs,
// reached through other deferred declarations.
func featuresBehind(
	graph entities.Graph,
	own ownership,
	start entities.DeclKey,
	memo map[entities.DeclKey]map[FeatureID]bool,
) map[FeatureID]bool {
	if found, ok := memo[start]; ok {
		return found
	}

	out := map[FeatureID]bool{}
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

			if owner := own.owner[edge.Provider]; owner != SharedFeature {
				out[owner] = true

				continue
			}

			if own.deferred[edge.Provider] {
				queue = append(queue, edge.Provider)
			}
		}
	}

	memo[start] = out

	return out
}

// earliest is the lowest position any of a feature's declarations reached. It
// is the tie-break between features neither of which needs the other, so a
// rebuilt history still follows the original chronology wherever the dependency
// graph leaves the choice open.
func earliest(own ownership, order entities.Ordering) map[FeatureID]entities.Position {
	out := map[FeatureID]entities.Position{}

	for key, feature := range own.owner {
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
