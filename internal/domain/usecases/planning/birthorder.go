package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// sortForBirth orders declarations as commits must carry them: by settled
// position, and WITHIN a position by dependency. The fixed point only
// guarantees a provider is not LATER, so provider and dependent routinely
// share a position, and a name tie-break inverts them.
func sortForBirth(
	graph entities.Graph,
	order entities.Ordering,
	keys []entities.DeclKey,
	groupOf func(entities.DeclKey) FeatureID,
) []entities.DeclKey {
	sorted := slices.Clone(keys)
	slices.SortFunc(sorted, func(a, b entities.DeclKey) int {
		if left, right := order.Of(a), order.Of(b); left != right {
			return cmp.Compare(left, right)
		}

		return cmp.Compare(a.String(), b.String())
	})

	out := make([]entities.DeclKey, 0, len(sorted))

	for start := 0; start < len(sorted); {
		end := start + 1
		for end < len(sorted) && order.Of(sorted[end]) == order.Of(sorted[start]) {
			end++
		}

		out = append(out, byDependency(graph, sorted[start:end], groupOf)...)
		start = end
	}

	return out
}

// byDependency topologically sorts one position's declarations. Only edges
// with BOTH ends inside the group count: one reaching outside is already
// satisfied by position. A cycle leaves members pending, keeping name order.
// Which topological order is picked matters - see nextReady.
//
// An atom is one node of the sort: its members are born in one commit, and the
// ready queue would otherwise interleave them with whatever else shares the
// position, stretching the atom over hundreds of declarations and every cut
// across it with it (a commit of 588 files, because no cut was left that split
// no atom).
func byDependency(
	graph entities.Graph,
	group []entities.DeclKey,
	groupOf func(entities.DeclKey) FeatureID,
) []entities.DeclKey {
	if len(group) < 2 {
		return group
	}

	nodes := atomNodes(graph, group)

	needs := make(map[entities.DeclKey][]entities.DeclKey, len(nodes.leaders))
	pending := make(map[entities.DeclKey]int, len(nodes.leaders))

	for _, key := range group {
		dependent := nodes.leaderOf[key]

		for _, edge := range graph.Needs(key) {
			provider, inside := nodes.leaderOf[edge.Provider]
			if !inside || provider == dependent {
				continue
			}

			needs[provider] = append(needs[provider], dependent)
			pending[dependent]++
		}
	}

	ordered := drain(nodes.leaders, needs, pending, groupOf)

	// Whatever a cycle left behind, in the order it came in.
	for _, leader := range nodes.leaders {
		if pending[leader] > 0 {
			ordered = append(ordered, leader)
		}
	}

	out := make([]entities.DeclKey, 0, len(group))
	for _, leader := range ordered {
		out = append(out, nodes.members[leader]...)
	}

	return out
}

// atomNodes folds a position's atoms into single nodes. The leader of a node is
// its first member in the group's order, so a declaration in no atom is its own
// leader and the order of the group is kept.
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

// drain runs the ready queue, emitting each declaration once everything it
// waits on inside the group has been emitted.
func drain(
	group []entities.DeclKey,
	needs map[entities.DeclKey][]entities.DeclKey,
	pending map[entities.DeclKey]int,
	groupOf func(entities.DeclKey) FeatureID,
) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(group))
	ready := make([]entities.DeclKey, 0, len(group))

	for _, key := range group {
		if pending[key] == 0 {
			ready = append(ready, key)
		}
	}

	current := FeatureID("")
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

// nextReady picks which of the declarations nothing separates goes next: the
// slice already being emitted, because switching costs a commit. Taking the
// first ready one instead - name order - doubled the commit count and spread a
// slice over 40% of the history.
func nextReady(
	ready []entities.DeclKey,
	groupOf func(entities.DeclKey) FeatureID,
	current FeatureID,
) int {
	if groupOf == nil {
		return 0
	}

	for index, candidate := range ready {
		if groupOf(candidate) == current {
			return index
		}
	}

	return 0
}
