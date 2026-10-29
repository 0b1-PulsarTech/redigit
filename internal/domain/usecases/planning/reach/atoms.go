package reach

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func AtomGroups(graph entities.Graph) map[string][]entities.DeclKey {
	groups := map[string][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		if decl.Atom == "" {
			continue
		}

		groups[decl.Atom] = append(groups[decl.Atom], decl.Key)
	}

	return groups
}

// AtomExtent maps each key's index to the index of the last member of its atom,
// or to its own when it belongs to none. An atom is born in one commit, and
// members that tied at one position arrive interleaved with unrelated
// declarations, so a cut between two of them is not a cut the plan may make.
func AtomExtent(graph entities.Graph, keys []entities.DeclKey) []int {
	last := map[string]int{}
	atomOf := make([]string, len(keys))

	for index, key := range keys {
		decl, found := graph.Decl(key)
		if !found || decl.Atom == "" {
			continue
		}

		atomOf[index] = decl.Atom
		last[decl.Atom] = index
	}

	extent := make([]int, len(keys))

	for index := range keys {
		extent[index] = index

		if atomOf[index] != "" {
			extent[index] = last[atomOf[index]]
		}
	}

	return extent
}
