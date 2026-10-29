package birth

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func SortForBirth(
	graph entities.Graph,
	order entities.Ordering,
	keys []entities.DeclKey,
	groupOf func(entities.DeclKey) reach.FeatureID,
) []entities.DeclKey {
	sorted := slices.Clone(keys)
	// The fixed point only guarantees a provider is not LATER, so provider and
	// dependent routinely share a position, and a name tie-break inverts them.
	slices.SortFunc(sorted, func(first, second entities.DeclKey) int {
		if left, right := order.Of(first), order.Of(second); left != right {
			return cmp.Compare(left, right)
		}

		return cmp.Compare(first.String(), second.String())
	})

	out := make([]entities.DeclKey, 0, len(sorted))
	subjects := reach.TestSubjects(graph)

	for start := 0; start < len(sorted); {
		end := start + 1
		for end < len(sorted) && order.Of(sorted[end]) == order.Of(sorted[start]) {
			end++
		}

		out = append(out, byDependency(graph, subjects, sorted[start:end], groupOf)...)
		start = end
	}

	return out
}

func byDependency(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	group []entities.DeclKey,
	groupOf func(entities.DeclKey) reach.FeatureID,
) []entities.DeclKey {
	if len(group) <= 1 {
		return group
	}

	nodes := atomNodes(graph, group)

	var links []link

	for _, key := range group {
		dependent := nodes.leaderOf[key]

		for _, edge := range graph.Needs(key) {
			provider, inside := nodes.leaderOf[edge.Provider]
			if !inside || provider == dependent {
				continue
			}

			links = append(
				links,
				link{provider: provider, dependent: dependent, soft: IsSoft(edge.Reason)},
			)
		}

		// A test shares its subject's position, and nothing but name order would
		// say which is born first.
		for _, subject := range subjects[key] {
			if provider, inside := nodes.leaderOf[subject]; inside && provider != dependent {
				links = append(links, link{provider: provider, dependent: dependent, soft: true})
			}
		}
	}

	ordered, stuck := drainLinks(nodes.leaders, links, groupOf, false)

	// A cycle that runs through a soft edge is not a cycle the code has: the
	// generator directive a mock is written from sits in the file that uses the
	// mock, and the mock is read by the code beside the directive.
	if len(stuck) > 0 {
		more, rest := drainLinks(stuck, links, groupOf, true)
		ordered = append(ordered, more...)
		stuck = rest
	}

	// Whatever a hard cycle left behind: the cycle, then what waits on it.
	ordered = append(ordered, inDependencyOrder(stuck, links)...)

	out := make([]entities.DeclKey, 0, len(group))
	for _, leader := range ordered {
		out = append(out, nodes.members[leader]...)
	}

	return out
}

type link struct {
	provider, dependent entities.DeclKey
	soft                bool
}

func IsSoft(reason entities.EdgeReason) bool {
	return reason == entities.ReasonGeneratedBy || reason == entities.ReasonMirrors
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

	// Taking the first ready one instead - name order - doubled the commit count
	// and spread a slice over 40% of the history.
	for index, candidate := range ready {
		if groupOf(candidate) == current {
			return index
		}
	}

	return 0
}
