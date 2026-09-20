package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func atomGroups(graph entities.Graph) map[string][]entities.DeclKey {
	groups := map[string][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		if decl.Atom == "" {
			continue
		}

		groups[decl.Atom] = append(groups[decl.Atom], decl.Key)
	}

	return groups
}
