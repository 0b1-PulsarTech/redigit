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
	// The ordering only guarantees a provider is not LATER, so provider and
	// dependent routinely share a position, and a name tie-break inverts them.
	slices.SortFunc(sorted, func(first, second entities.DeclKey) int {
		if left, right := order.Of(first), order.Of(second); left != right {
			return cmp.Compare(left, right)
		}

		return entities.CompareKeys(first, second)
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

	ordered, stuck := drainAllLinks(nodes.leaders, links, groupOf)

	// A cycle that runs through a soft edge is not a cycle the code has: the
	// generator directive a mock is written from sits in the file that uses the
	// mock, and the mock is read by the code beside the directive.
	if len(stuck) > 0 {
		more, rest := drainHardLinks(stuck, links, groupOf)
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

func IsSoft(reason entities.EdgeReason) bool {
	return reason == entities.ReasonGeneratedBy || reason == entities.ReasonMirrors
}
