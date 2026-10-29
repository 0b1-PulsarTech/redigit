package testplace

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func PlaceTests(graph entities.Graph, keys []entities.DeclKey) []entities.DeclKey {
	place := newPlacement(graph, keys)

	// A move never passes a dependency and never splits an atom.
	for _, key := range keys {
		place.consider(key)
	}

	place.settleAtoms()

	if len(place.moved) == 0 {
		return gatherTestAtoms(graph, keys)
	}

	out := make([]entities.DeclKey, 0, len(keys))

	for at, key := range keys {
		if !place.moved[key] {
			out = append(out, key)
		}

		out = append(out, place.after[at]...)
	}

	return gatherTestAtoms(graph, out)
}

func gatherTestAtoms(graph entities.Graph, keys []entities.DeclKey) []entities.DeclKey {
	index := make(map[entities.DeclKey]int, len(keys))
	// A member moves only when nothing placed between it and the last member
	// needs it.
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
