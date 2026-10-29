package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func proseEdge(dependent, provider entities.Decl) entities.Edge {
	return entities.Edge{
		Dependent: dependent.Key,
		Provider:  provider.Key,
		Reason:    entities.ReasonProse,
	}
}

func keysOf(decls []entities.Decl) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(decls))
	for _, item := range decls {
		out = append(out, item.Key)
	}

	return out
}
