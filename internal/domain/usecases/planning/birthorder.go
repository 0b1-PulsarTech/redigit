package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// sortForBirth orders declarations the way commits must carry them: by the
// settled position, and WITHIN one position by dependency. The fixed point
// only guarantees a provider is not LATER, so a provider and its dependent
// routinely share a position, and a tie-break that is not the dependency can
// invert them - by name, a go.work "use" landed 19 commits early.
func sortForBirth(
	graph entities.Graph,
	order entities.Ordering,
	keys []entities.DeclKey,
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

		out = append(out, byDependency(graph, sorted[start:end])...)
		start = end
	}

	return out
}

// byDependency topologically sorts one position's declarations, keeping name
// order among those nothing separates. Only edges with BOTH ends inside the
// group count: one reaching outside is already satisfied by position. A cycle
// leaves members pending, and those keep name order - two that wait on each
// other share a position and cannot be separated anyway.
func byDependency(graph entities.Graph, group []entities.DeclKey) []entities.DeclKey {
	if len(group) < 2 {
		return group
	}

	inside := make(map[entities.DeclKey]bool, len(group))
	for _, key := range group {
		inside[key] = true
	}

	needs := make(map[entities.DeclKey][]entities.DeclKey, len(group))
	pending := make(map[entities.DeclKey]int, len(group))

	for _, key := range group {
		for _, edge := range graph.Needs(key) {
			if !inside[edge.Provider] || edge.Provider == key {
				continue
			}

			needs[edge.Provider] = append(needs[edge.Provider], key)
			pending[key]++
		}
	}

	out := make([]entities.DeclKey, 0, len(group))
	ready := make([]entities.DeclKey, 0, len(group))

	for _, key := range group {
		if pending[key] == 0 {
			ready = append(ready, key)
		}
	}

	for len(ready) > 0 {
		key := ready[0]
		ready = ready[1:]
		out = append(out, key)

		for _, dependent := range needs[key] {
			pending[dependent]--
			if pending[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}

	// Whatever a cycle left behind, in the order it came in.
	for _, key := range group {
		if pending[key] > 0 {
			out = append(out, key)
		}
	}

	return out
}
