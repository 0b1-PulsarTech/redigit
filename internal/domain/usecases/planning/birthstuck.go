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
	index := make(map[entities.DeclKey]int, len(stuck))
	for at, node := range stuck {
		index[node] = at
	}

	providers := make(map[entities.DeclKey][]entities.DeclKey, len(stuck))

	for _, wait := range links {
		_, dependentIn := index[wait.dependent]
		_, providerIn := index[wait.provider]

		if dependentIn && providerIn && !wait.soft {
			providers[wait.dependent] = append(providers[wait.dependent], wait.provider)
		}
	}

	walk := componentWalk{
		providers: providers,
		low:       map[entities.DeclKey]int{},
		number:    map[entities.DeclKey]int{},
		onStack:   map[entities.DeclKey]bool{},
	}

	for _, node := range stuck {
		if _, seen := walk.number[node]; !seen {
			walk.visit(node)
		}
	}

	return walk.ordered
}

// componentWalk is Tarjan's walk from a dependent to its providers, which
// closes a component only after every component it waits on.
type componentWalk struct {
	providers map[entities.DeclKey][]entities.DeclKey
	low       map[entities.DeclKey]int
	number    map[entities.DeclKey]int
	onStack   map[entities.DeclKey]bool
	stack     []entities.DeclKey
	ordered   []entities.DeclKey
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

	walk.ordered = append(walk.ordered, walk.stack[start:]...)
	walk.stack = walk.stack[:start]
}
