package slicing

import (
	"go/types"
	"maps"
	"slices"
)

type groups struct{ parent []int }

func newGroups(size int) groups {
	parent := make([]int, size)
	for index := range parent {
		parent[index] = index
	}

	return groups{parent: parent}
}

func (set groups) find(index int) int {
	for set.parent[index] != index {
		set.parent[index] = set.parent[set.parent[index]]
		index = set.parent[index]
	}

	return index
}

func (set groups) join(left, right int) {
	rootA, rootB := set.find(left), set.find(right)
	if rootA != rootB {
		set.parent[max(rootA, rootB)] = min(rootA, rootB)
	}
}

func (set groups) size(root int) int {
	count := 0

	for index := range set.parent {
		if set.find(index) == root {
			count++
		}
	}

	return count
}

func (flow bodyFlow) joinDeclarations(groups groups) {
	for index := range flow.declares {
		for _, obj := range slices.SortedFunc(maps.Keys(flow.declares[index]), byPos) {
			flow.joinReader(groups, index, obj)
		}
	}
}

func (flow bodyFlow) joinReader(groups groups, declaration int, obj types.Object) {
	// A declaration is born with the first statement that reads it: Go refuses an unused local.
	first := -1

	for index := declaration + 1; index < len(flow.reads); index++ {
		if !flow.reads[index][obj] {
			continue
		}

		if groups.find(index) == groups.find(declaration) {
			return
		}

		if first < 0 {
			first = index
		}
	}

	if first >= 0 {
		groups.join(declaration, first)
	}
}
