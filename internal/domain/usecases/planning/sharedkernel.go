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

	index := sharedIndex{
		atoms:    atomGroups(graph),
		testsOf:  testAnchors(graph),
		subjects: testSubjects(graph),
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

	return sortedKeys(wanted)
}

// sharedIndex is the shared-kernel structure a single unbornProviders walk
// leans on beyond graph.Needs: which declarations a generated file wrote
// together (atoms), and which test an Anchor points at each of them.
type sharedIndex struct {
	atoms   map[string][]entities.DeclKey
	testsOf map[entities.DeclKey][]entities.DeclKey
	// subjects is, per test, what it exercises: the reverse of testsOf.
	subjects map[entities.DeclKey][]entities.DeclKey
	// walked is what the walk has taken into the shared commit so far.
	walked map[entities.DeclKey]bool
	own    ownership
}

// siblingsOf is everything current's place in the shared kernel drags along
// with it: the providers it needs, the atom it was written in, and any test
// anchored to it that is itself already safe to pull.
func (index sharedIndex) siblingsOf(
	graph entities.Graph,
	own ownership,
	born map[entities.DeclKey]bool,
	current entities.DeclKey,
) []entities.DeclKey {
	var out []entities.DeclKey

	// Whatever a shared declaration needs is reached by every feature that
	// reaches the declaration itself (see assign), so it is shared too: the
	// walk never waits on a feature not yet processed.
	for _, edge := range graph.Needs(current) {
		if own.owner[edge.Provider] == SharedFeature && !driftsToTest(graph, edge) {
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
		if own.owner[test] == SharedFeature && index.safeToPull(graph, test, born) {
			out = append(out, test)
		}
	}

	return out
}

// packageOf is the rest of the package of a shared declaration a feature reads:
// a package is one importable thing, so the first feature to need any of it
// brings the rest instead of growing it one consumer at a time.
func (index sharedIndex) packageOf(
	graph entities.Graph,
	current entities.DeclKey,
) []entities.DeclKey {
	decl, found := graph.Decl(current)
	if !found || index.own.owner[current] != SharedFeature {
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
