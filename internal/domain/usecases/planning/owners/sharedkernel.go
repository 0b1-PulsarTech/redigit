package owners

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func UnbornProviders(
	graph entities.Graph,
	own Ownership,
	members []entities.DeclKey,
	born map[entities.DeclKey]bool,
) []entities.DeclKey {
	wanted := map[entities.DeclKey]bool{}
	inFeature := map[entities.DeclKey]bool{}

	for _, key := range members {
		inFeature[key] = true
	}

	index := sharedIndex{
		atoms:    reach.AtomGroups(graph),
		testsOf:  testAnchors(graph),
		subjects: reach.TestSubjects(graph),
		walked:   wanted,
		own:      own,
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

		for _, key := range index.siblingsOf(graph, own, born, current) {
			add(key)
		}

		for _, key := range index.packageOf(graph, current) {
			add(key)
		}
	}

	return SortedKeys(wanted)
}

type sharedIndex struct {
	atoms   map[string][]entities.DeclKey
	testsOf map[entities.DeclKey][]entities.DeclKey
	// subjects is, per test, what it exercises: the reverse of testsOf.
	subjects map[entities.DeclKey][]entities.DeclKey
	// walked is what the walk has taken into the shared commit so far.
	walked map[entities.DeclKey]bool
	own    Ownership
}

func (index sharedIndex) siblingsOf(
	graph entities.Graph,
	own Ownership,
	born map[entities.DeclKey]bool,
	current entities.DeclKey,
) []entities.DeclKey {
	var out []entities.DeclKey

	// Whatever a shared declaration needs is reached by every feature that
	// reaches the declaration itself (see assign), so it is shared too: the
	// walk never waits on a feature not yet processed.
	foreign := index.takesForeign(own, current)

	for _, edge := range graph.Needs(current) {
		if (foreign || own.Owner[edge.Provider] == reach.SharedFeature) &&
			!driftsToTest(graph, edge) && !entities.FillsInterface(graph, edge) {
			out = append(out, edge.Provider)
		}
	}

	// A generated file's declarations carry no edge to each other, only the
	// Atom they share, so a sibling is folded in by hand or one half of the
	// file is born a commit its own reader never asked for.
	if decl, found := graph.Decl(current); found && decl.Atom != "" {
		out = append(out, index.atoms[decl.Atom]...)
	}

	// Nothing needs a test, so only the Anchor points to it; pulling it in
	// early is safe only once its own needs are already met (born, or shared
	// and walked by this same closure).
	for _, test := range index.testsOf[current] {
		if own.Owner[test] == reach.SharedFeature && index.safeToPull(graph, test, born) {
			out = append(out, test)
		}
	}

	return out
}

func (index sharedIndex) packageOf(
	graph entities.Graph,
	current entities.DeclKey,
) []entities.DeclKey {
	decl, found := graph.Decl(current)
	if !found || index.own.Owner[current] != reach.SharedFeature {
		return nil
	}

	return index.own.packages[decl.Unit]
}

func testAnchors(graph entities.Graph) map[entities.DeclKey][]entities.DeclKey {
	out := map[entities.DeclKey][]entities.DeclKey{}

	for _, anchor := range graph.Anchors() {
		if anchor.Reason == entities.ReasonTestOf {
			out[anchor.To] = append(out[anchor.To], anchor.Decl)
		}
	}

	return out
}

func Remaining(graph entities.Graph, born map[entities.DeclKey]bool) []entities.DeclKey {
	left := map[entities.DeclKey]bool{}

	for _, decl := range graph.Decls() {
		if !born[decl.Key] {
			left[decl.Key] = true
		}
	}

	return SortedKeys(left)
}

func SortedKeys(set map[entities.DeclKey]bool) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(set))
	for key := range set {
		out = append(out, key)
	}

	slices.SortFunc(out, entities.CompareKeys)

	return out
}

func MarkBorn(born map[entities.DeclKey]bool, keys []entities.DeclKey) {
	for _, key := range keys {
		born[key] = true
	}
}
