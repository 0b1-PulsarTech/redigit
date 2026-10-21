package planning

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// inDependencyOrder arranges what a hard cycle kept back so that whatever
// waits on a cycle comes after it. Draining stops at the cycle and leaves its
// dependents pending too; kept in arrival order, a table that points at a
// cycle member was born ahead of it, by name.
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

// componentsOf is the strongly connected components of nodes, each after every
// component it waits on, with the members of one in the order they came in.
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

// componentWalk is Tarjan's walk from a dependent to its providers, which
// closes a component only after every component it waits on.
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

// cycleExtent widens the extent of every member of a hard cycle to the cycle's
// last member: mutual references compile in one commit or in none, so a cut
// between two of them is not a cut the plan may make.
func cycleExtent(graph entities.Graph, keys []entities.DeclKey, extent []int) []int {
	at := make(map[entities.DeclKey]int, len(keys))
	for index, key := range keys {
		at[key] = index
	}

	providers := map[entities.DeclKey][]entities.DeclKey{}

	for _, key := range keys {
		for _, edge := range graph.Needs(key) {
			if _, inside := at[edge.Provider]; inside && !isSoft(edge.Reason) {
				providers[key] = append(providers[key], edge.Provider)
			}
		}
	}

	for _, component := range componentsOf(keys, providers) {
		if len(component) == 1 {
			continue
		}

		last := 0
		for _, key := range component {
			last = max(last, extent[at[key]])
		}

		for _, key := range component {
			extent[at[key]] = max(extent[at[key]], last)
		}
	}

	return extent
}
