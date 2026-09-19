package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// unbornProviders is what a feature's members wait on that no commit has
// written yet, closed TRANSITIVELY over the shared kernel: a shared
// declaration pulled in for being needed also pulls in whatever IT needs.
func unbornProviders(
	graph entities.Graph,
	own ownership,
	members []entities.DeclKey,
	born map[entities.DeclKey]bool,
) []entities.DeclKey {
	wanted := map[entities.DeclKey]bool{}
	inFeature := map[entities.DeclKey]bool{}

	for _, key := range members {
		inFeature[key] = true
	}

	queue := slices.Clone(members)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range graph.Needs(current) {
			if born[edge.Provider] || inFeature[edge.Provider] || wanted[edge.Provider] {
				continue
			}

			// Whatever a shared declaration needs is reached by every feature
			// that reaches the declaration itself (see assign), so it is
			// shared too: the walk never waits on a feature not yet processed.
			if own.owner[edge.Provider] != SharedFeature {
				continue
			}

			wanted[edge.Provider] = true

			queue = append(queue, edge.Provider)
		}
	}

	return sortedKeys(wanted)
}

func remaining(graph entities.Graph, born map[entities.DeclKey]bool) []entities.DeclKey {
	left := map[entities.DeclKey]bool{}

	for _, decl := range graph.Decls() {
		if !born[decl.Key] {
			left[decl.Key] = true
		}
	}

	return sortedKeys(left)
}

func sortedKeys(set map[entities.DeclKey]bool) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(set))
	for key := range set {
		out = append(out, key)
	}

	slices.SortFunc(
		out,
		func(a, b entities.DeclKey) int { return cmp.Compare(a.String(), b.String()) },
	)

	return out
}

func markBorn(born map[entities.DeclKey]bool, keys []entities.DeclKey) {
	for _, key := range keys {
		born[key] = true
	}
}
