package slicing

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
