package birth

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func inDependencyOrder(stuck []entities.DeclKey, links []link) []entities.DeclKey {
	providers := map[entities.DeclKey][]entities.DeclKey{}
	inside := make(map[entities.DeclKey]bool, len(stuck))

	for _, node := range stuck {
		inside[node] = true
	}

	for _, wait := range links {
		if inside[wait.dependent] && inside[wait.provider] && !wait.soft {
			providers[wait.dependent] = append(providers[wait.dependent], wait.provider)
		}
	}

	var ordered []entities.DeclKey

	for _, component := range componentsOf(stuck, providers) {
		ordered = append(ordered, component...)
	}

	return ordered
}

func componentsOf(
	nodes []entities.DeclKey,
	providers map[entities.DeclKey][]entities.DeclKey,
) [][]entities.DeclKey {
	walk := componentWalk{
		providers: providers,
		low:       map[entities.DeclKey]int{},
		number:    map[entities.DeclKey]int{},
		onStack:   map[entities.DeclKey]bool{},
	}

	for _, node := range nodes {
		if _, seen := walk.number[node]; !seen {
			walk.visit(node)
		}
	}

	return walk.components
}

type componentWalk struct {
	providers  map[entities.DeclKey][]entities.DeclKey
	low        map[entities.DeclKey]int
	number     map[entities.DeclKey]int
	onStack    map[entities.DeclKey]bool
	stack      []entities.DeclKey
	components [][]entities.DeclKey
}

func (walk *componentWalk) visit(node entities.DeclKey) {
	walk.number[node] = len(walk.number) + 1
	walk.low[node] = walk.number[node]
	walk.stack = append(walk.stack, node)
	walk.onStack[node] = true

	for _, provider := range walk.providers[node] {
		if _, seen := walk.number[provider]; !seen {
			walk.visit(provider)

			walk.low[node] = min(walk.low[node], walk.low[provider])

			continue
		}

		if walk.onStack[provider] {
			walk.low[node] = min(walk.low[node], walk.number[provider])
		}
	}

	if walk.low[node] != walk.number[node] {
		return
	}

	start := slices.Index(walk.stack, node)

	for _, member := range walk.stack[start:] {
		walk.onStack[member] = false
	}

	walk.components = append(walk.components, slices.Clone(walk.stack[start:]))
	walk.stack = walk.stack[:start]
}

func CycleExtent(graph entities.Graph, keys []entities.DeclKey, extent []int) []int {
	nodes := atomNodes(graph, keys)
	providers := map[entities.DeclKey][]entities.DeclKey{}

	for _, key := range keys {
		dependent := nodes.leaderOf[key]

		for _, edge := range graph.Needs(key) {
			provider, inside := nodes.leaderOf[edge.Provider]
			if inside && provider != dependent && !IsSoft(edge.Reason) {
				providers[dependent] = append(providers[dependent], provider)
			}
		}
	}

	at := make(map[entities.DeclKey]int, len(keys))
	for index, key := range keys {
		at[key] = index
	}

	for _, component := range componentsOf(nodes.leaders, providers) {
		if len(component) == 1 {
			continue
		}

		var members []entities.DeclKey

		last := 0

		for _, leader := range component {
			members = append(members, nodes.members[leader]...)
		}

		for _, key := range members {
			last = max(last, extent[at[key]])
		}

		for _, key := range members {
			extent[at[key]] = max(extent[at[key]], last)
		}
	}

	return extent
}
