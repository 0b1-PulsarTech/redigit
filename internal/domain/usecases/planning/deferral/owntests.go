package deferral

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func OwnTests(
	graph entities.Graph,
	own owners.Ownership,
	born, fresh map[entities.DeclKey]bool,
) []entities.DeclKey {
	subjects := reach.TestSubjects(graph)
	taken := map[entities.DeclKey]bool{}
	atoms := reach.AtomGroups(graph)

	for _, key := range readyWithin(graph, own, atoms, born, fresh) {
		if !own.Roots[key] && !born[key] {
			taken[key] = true
		}
	}

	// An atom is born in one commit: a member held back holds its mates back.
	held := func(mate entities.DeclKey) bool {
		return !born[mate] && !taken[mate]
	}

	read := func(provider entities.DeclKey) bool { return !born[provider] && !taken[provider] }
	// Subjects keep the Deferred clause: a twin owned by a later feature that the test
	// does not read must not pull it off its subject's row.
	exercised := func(provider entities.DeclKey) bool {
		return read(provider) && own.Deferred[provider]
	}

	for changed := true; changed; {
		changed = false
		memo := map[string]bool{}

		for key := range taken {
			if !waitsOn(graph, nil, key, read) && !slices.ContainsFunc(subjects[key], exercised) &&
				!atomBlocked(graph, atoms, key, memo, held) {
				continue
			}

			delete(taken, key)

			changed = true
		}
	}

	return owners.SortedKeys(taken)
}
