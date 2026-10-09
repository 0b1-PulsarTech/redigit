package planning

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// placeTests moves each test-side declaration to just behind the last thing it
// needs or exercises in the same commit group, so a suite is read with its
// subject. The fixed point lands tests wherever their position falls, which is
// often a whole feature after the code under test and past the cut that would
// have kept the two together.
//
// A move never passes a dependency (the test goes after every provider in the
// group) and never splits an atom (the test goes after the atom's last member).
func placeTests(graph entities.Graph, keys []entities.DeclKey) []entities.DeclKey {
	index := make(map[entities.DeclKey]int, len(keys))
	for at, key := range keys {
		index[key] = at
	}

	subjects := testSubjects(graph)
	lastOf := atomExtent(graph, keys)
	after := map[int][]entities.DeclKey{}
	moved := map[entities.DeclKey]bool{}
	slot := map[entities.DeclKey]int{}

	for _, key := range keys {
		decl, found := graph.Decl(key)
		if !found || !isTestSide(decl) {
			continue
		}

		anchor := -1

		raise := func(provider entities.DeclKey) {
			if at, placed := slot[provider]; placed {
				anchor = max(anchor, at)

				return
			}

			if at, inside := index[provider]; inside && provider != key {
				anchor = max(anchor, lastOf[at])
			}
		}

		for _, edge := range graph.Needs(key) {
			raise(edge.Provider)
		}

		for _, subject := range subjects[key] {
			raise(subject)
		}

		if anchor < 0 || anchor+1 >= index[key] {
			continue
		}

		moved[key] = true
		slot[key] = anchor
		after[anchor] = append(after[anchor], key)
	}

	if len(moved) == 0 {
		return keys
	}

	out := make([]entities.DeclKey, 0, len(keys))

	for at, key := range keys {
		if !moved[key] {
			out = append(out, key)
		}

		out = append(out, after[at]...)
	}

	return out
}
