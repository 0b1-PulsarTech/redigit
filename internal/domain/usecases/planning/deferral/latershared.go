package deferral

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// SplitLate parts what the shared commit would bring into what it can carry
// alone and what reads the feature's own members. A walked-in statement of the
// assembly reaches the feature's code, and born in the commit before the
// feature it would name a declaration that does not exist yet; it goes with
// the feature, in the commit that births what it reads.
func SplitLate(
	graph entities.Graph,
	shared, members []entities.DeclKey,
) (early, late []entities.DeclKey) {
	waiting := map[entities.DeclKey]bool{}
	for _, key := range members {
		waiting[key] = true
	}

	atoms := reach.AtomGroups(graph)

	for changed := true; changed; {
		changed = false

		for _, key := range shared {
			if waiting[key] {
				continue
			}

			if !readsAny(graph, key, waiting) && !atomWaiting(graph, atoms, key, waiting) {
				continue
			}

			waiting[key] = true
			changed = true
		}
	}

	for _, key := range shared {
		if waiting[key] {
			late = append(late, key)

			continue
		}

		early = append(early, key)
	}

	return early, slices.Clip(late)
}

func readsAny(graph entities.Graph, key entities.DeclKey, set map[entities.DeclKey]bool) bool {
	for _, edge := range graph.Needs(key) {
		if set[edge.Provider] {
			return true
		}
	}

	return false
}

// atomWaiting is whether another member of the declaration's atom waits: an
// atom is born in one commit, so it goes late as one.
func atomWaiting(
	graph entities.Graph,
	atoms map[string][]entities.DeclKey,
	key entities.DeclKey,
	waiting map[entities.DeclKey]bool,
) bool {
	return slices.ContainsFunc(atomMembers(graph, atoms, key), func(member entities.DeclKey) bool {
		return waiting[member]
	})
}
