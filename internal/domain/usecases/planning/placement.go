package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// placement is the moves placeTests has decided so far: after[at] are the
// declarations to land right behind keys[at].
type placement struct {
	graph    entities.Graph
	index    map[entities.DeclKey]int
	subjects map[entities.DeclKey][]entities.DeclKey
	lastOf   []int
	after    map[int][]entities.DeclKey
	moved    map[entities.DeclKey]bool
	slot     map[entities.DeclKey]int

	// An atom is placed as one: its members land behind the last thing ANY of
	// them needs, or stay put, so a generated file is never stretched over the
	// group by each member finding its own anchor.
	atoms   []string
	members map[string][]entities.DeclKey
	anchor  map[string]int
	first   map[string]int
}

func newPlacement(graph entities.Graph, keys []entities.DeclKey) *placement {
	index := make(map[entities.DeclKey]int, len(keys))
	for at, key := range keys {
		index[key] = at
	}

	return &placement{
		graph:    graph,
		index:    index,
		subjects: testSubjects(graph),
		lastOf:   atomExtent(graph, keys),
		after:    map[int][]entities.DeclKey{},
		moved:    map[entities.DeclKey]bool{},
		slot:     map[entities.DeclKey]int{},
		members:  map[string][]entities.DeclKey{},
		anchor:   map[string]int{},
		first:    map[string]int{},
	}
}

func (place *placement) consider(key entities.DeclKey) {
	decl, found := place.graph.Decl(key)
	if !found || !entities.IsTestSide(decl) {
		return
	}

	anchor := place.anchorOf(decl)

	if decl.Atom != "" {
		place.join(decl.Atom, key, anchor)

		return
	}

	place.move(key, anchor, place.index[key])
}

func (place *placement) join(atom string, key entities.DeclKey, anchor int) {
	if _, seen := place.first[atom]; !seen {
		place.first[atom] = place.index[key]
		place.anchor[atom] = -1
		place.atoms = append(place.atoms, atom)
	}

	place.members[atom] = append(place.members[atom], key)
	place.anchor[atom] = max(place.anchor[atom], anchor)
}

func (place *placement) settleAtoms() {
	for _, atom := range place.atoms {
		for _, key := range place.members[atom] {
			place.move(key, place.anchor[atom], place.first[atom])
		}
	}
}

// move lands key behind the anchor, unless it is already there or nothing
// anchors it.
func (place *placement) move(key entities.DeclKey, anchor, from int) {
	if anchor < 0 || anchor+1 >= from {
		return
	}

	place.moved[key] = true
	place.slot[key] = anchor
	place.after[anchor] = append(place.after[anchor], key)
}

// anchorOf is the index of the last thing the declaration needs or exercises
// that sits in the group, outside its own atom.
func (place *placement) anchorOf(decl entities.Decl) int {
	anchor := -1

	raise := func(provider entities.DeclKey) {
		if peer, found := place.graph.Decl(provider); found && decl.Atom != "" &&
			peer.Atom == decl.Atom {
			return
		}

		if at, placed := place.slot[provider]; placed {
			anchor = max(anchor, at)

			return
		}

		if at, inside := place.index[provider]; inside && provider != decl.Key {
			anchor = max(anchor, place.lastOf[at])
		}
	}

	for _, edge := range place.graph.Needs(decl.Key) {
		raise(edge.Provider)
	}

	for _, subject := range place.subjects[decl.Key] {
		raise(subject)
	}

	return anchor
}
