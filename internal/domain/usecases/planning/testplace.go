package planning

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// placeTests moves each test-side declaration to just behind the last thing it
// needs or exercises in the same commit group, so a suite is read with its
// subject. A move never passes a dependency and never splits an atom.
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
		if !found || !entities.IsTestSide(decl) {
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

// gatherTestAtoms brings the members of a test-side atom up to its last member,
// so the atom is not stretched over a commit group. A member moves only when
// nothing placed between it and the last member needs it.
func gatherTestAtoms(graph entities.Graph, keys []entities.DeclKey) []entities.DeclKey {
	index := make(map[entities.DeclKey]int, len(keys))
	atomOf := make(map[entities.DeclKey]string, len(keys))
	last := map[string]int{}

	for at, key := range keys {
		index[key] = at

		decl, found := graph.Decl(key)
		if !found || decl.Atom == "" || !entities.IsTestSide(decl) {
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

		if neededBetween(graph, index, atomOf, key, span{from: at, to: last[atom]}) {
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

type span struct{ from, to int }

func neededBetween(
	graph entities.Graph,
	index map[entities.DeclKey]int,
	atomOf map[entities.DeclKey]string,
	key entities.DeclKey,
	within span,
) bool {
	for _, edge := range graph.Consumers(key) {
		at, inside := index[edge.Dependent]
		if inside && at > within.from && at <= within.to && atomOf[edge.Dependent] != atomOf[key] {
			return true
		}
	}

	return false
}
