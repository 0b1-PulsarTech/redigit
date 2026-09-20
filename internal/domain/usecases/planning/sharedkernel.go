package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

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

	atoms := atomGroups(graph)
	atomOf := map[entities.DeclKey]string{}

	for atom, keys := range atoms {
		for _, key := range keys {
			atomOf[key] = atom
		}
	}

	queue := slices.Clone(members)

	add := func(key entities.DeclKey) {
		if born[key] || inFeature[key] || wanted[key] {
			return
		}

		wanted[key] = true

		queue = append(queue, key)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range graph.Needs(current) {
			// Whatever a shared declaration needs is reached by every feature
			// that reaches the declaration itself (see assign), so it is
			// shared too: the walk never waits on a feature not yet processed.
			if own.owner[edge.Provider] != SharedFeature {
				continue
			}

			add(edge.Provider)
		}

		// A generated file's declarations carry no edge to each other, only
		// the Atom they share, so the walk has to fold in a sibling by hand or
		// one half of the file is born a commit its own reader never asked for.
		for _, sibling := range atoms[atomOf[current]] {
			add(sibling)
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
