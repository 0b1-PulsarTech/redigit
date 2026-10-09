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
		return gatherTestAtoms(graph, keys)
	}

	out := make([]entities.DeclKey, 0, len(keys))

	for at, key := range keys {
		if !moved[key] {
			out = append(out, key)
		}

		out = append(out, after[at]...)
	}

	return gatherTestAtoms(graph, out)
}

// gatherTestAtoms brings the members of an atom made only of test-side
// declarations up to its last member. The fixed point places TestMain and the
// first test of a package far apart, and an atom stretched over a whole commit
// group leaves no place to cut it.
//
// A member moves only when nothing placed between it and the last member
// needs it; moving it later never passes something it needs.
func gatherTestAtoms(graph entities.Graph, keys []entities.DeclKey) []entities.DeclKey {
	index := make(map[entities.DeclKey]int, len(keys))
	atomOf := make(map[entities.DeclKey]string, len(keys))
	last := map[string]int{}

	for at, key := range keys {
		index[key] = at

		decl, found := graph.Decl(key)
		if !found || decl.Atom == "" || !isTestSide(decl) {
			continue
		}

		atomOf[key] = decl.Atom
		last[decl.Atom] = at
	}

	held := map[int][]entities.DeclKey{}
	delayed := map[entities.DeclKey]bool{}

	for at, key := range keys {
		atom, grouped := atomOf[key]
		if !grouped || at == last[atom] {
			continue
		}

		if neededBetween(graph, index, atomOf, key, at, last[atom]) {
			continue
		}

		held[last[atom]] = append(held[last[atom]], key)
		delayed[key] = true
	}

	if len(delayed) == 0 {
		return keys
	}

	out := make([]entities.DeclKey, 0, len(keys))

	for at, key := range keys {
		if delayed[key] {
			continue
		}

		out = append(out, held[at]...)
		out = append(out, key)
	}

	return out
}

func neededBetween(
	graph entities.Graph,
	index map[entities.DeclKey]int,
	atomOf map[entities.DeclKey]string,
	key entities.DeclKey,
	from, to int,
) bool {
	for _, edge := range graph.Consumers(key) {
		at, inside := index[edge.Dependent]
		if inside && at > from && at <= to && atomOf[edge.Dependent] != atomOf[key] {
			return true
		}
	}

	return false
}
