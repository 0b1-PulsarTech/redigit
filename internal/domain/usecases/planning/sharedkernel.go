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

	index := sharedIndex{atoms: atomGroups(graph), testsOf: testAnchors(graph), own: own}
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
	}

	return sortedKeys(wanted)
}

// sharedIndex is the shared-kernel structure a single unbornProviders walk
// leans on beyond graph.Needs: which declarations a generated file wrote
// together (atoms), and which test an Anchor points at each of them.
type sharedIndex struct {
	atoms   map[string][]entities.DeclKey
	testsOf map[entities.DeclKey][]entities.DeclKey
	own     ownership
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
		if own.owner[edge.Provider] == SharedFeature {
			out = append(out, edge.Provider)
		}
	}

	// A library is born whole by its first consumer.
	out = append(out, index.own.libraries[index.own.library[current]]...)

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
		if own.owner[test] == SharedFeature && safeToPullNow(graph, test, own, born) {
			out = append(out, test)
		}
	}

	return out
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

func safeToPullNow(
	graph entities.Graph,
	key entities.DeclKey,
	own ownership,
	born map[entities.DeclKey]bool,
) bool {
	for _, edge := range graph.Needs(key) {
		if !born[edge.Provider] && own.owner[edge.Provider] != SharedFeature {
			return false
		}
	}

	return true
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
