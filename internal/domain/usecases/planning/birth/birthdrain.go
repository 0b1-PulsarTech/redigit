package birth

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

type link struct {
	provider, dependent entities.DeclKey
	soft                bool
}

func drainLinks(
	nodes []entities.DeclKey,
	links []link,
	groupOf func(entities.DeclKey) reach.FeatureID,
	hardOnly bool,
) (emitted, stuck []entities.DeclKey) {
	inside := make(map[entities.DeclKey]bool, len(nodes))
	for _, node := range nodes {
		inside[node] = true
	}

	needs := make(map[entities.DeclKey][]entities.DeclKey, len(nodes))
	pending := make(map[entities.DeclKey]int, len(nodes))

	for _, wait := range links {
		if !inside[wait.provider] || !inside[wait.dependent] || hardOnly && wait.soft {
			continue
		}

		needs[wait.provider] = append(needs[wait.provider], wait.dependent)
		pending[wait.dependent]++
	}

	emitted = drain(nodes, needs, pending, groupOf)

	for _, node := range nodes {
		if pending[node] > 0 {
			stuck = append(stuck, node)
		}
	}

	return emitted, stuck
}

type atomNodeSet struct {
	leaders  []entities.DeclKey
	leaderOf map[entities.DeclKey]entities.DeclKey
	members  map[entities.DeclKey][]entities.DeclKey
}

func atomNodes(graph entities.Graph, group []entities.DeclKey) atomNodeSet {
	set := atomNodeSet{
		leaderOf: make(map[entities.DeclKey]entities.DeclKey, len(group)),
		members:  make(map[entities.DeclKey][]entities.DeclKey, len(group)),
	}

	byAtom := map[string]entities.DeclKey{}

	for _, key := range group {
		leader := key

		if decl, found := graph.Decl(key); found && decl.Atom != "" {
			first, seen := byAtom[decl.Atom]
			if !seen {
				byAtom[decl.Atom] = key
				first = key
			}

			leader = first
		}

		if leader == key {
			set.leaders = append(set.leaders, key)
		}

		set.leaderOf[key] = leader
		set.members[leader] = append(set.members[leader], key)
	}

	return set
}

func drain(
	group []entities.DeclKey,
	needs map[entities.DeclKey][]entities.DeclKey,
	pending map[entities.DeclKey]int,
	groupOf func(entities.DeclKey) reach.FeatureID,
) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(group))
	ready := make([]entities.DeclKey, 0, len(group))

	for _, key := range group {
		if pending[key] == 0 {
			ready = append(ready, key)
		}
	}

	current := reach.FeatureID("")
	started := false

	for len(ready) > 0 {
		pick := 0
		if started {
			pick = nextReady(ready, groupOf, current)
		}

		key := ready[pick]
		ready = append(ready[:pick], ready[pick+1:]...)
		out = append(out, key)

		if groupOf != nil {
			current = groupOf(key)
			started = true
		}

		for _, dependent := range needs[key] {
			pending[dependent]--
			if pending[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}

	return out
}

func nextReady(
	ready []entities.DeclKey,
	groupOf func(entities.DeclKey) reach.FeatureID,
	current reach.FeatureID,
) int {
	if groupOf == nil {
		return 0
	}

	// Staying in the current group matters: taking the first ready one by name
	// order doubled the commit count and spread a slice over 40% of the history.
	for index, candidate := range ready {
		if groupOf(candidate) == current {
			return index
		}
	}

	return 0
}
