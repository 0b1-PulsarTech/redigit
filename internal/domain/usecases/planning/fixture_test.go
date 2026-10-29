package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func edgeOf(dependent, provider entities.Decl, reason entities.EdgeReason) entities.Edge {
	return entities.Edge{Dependent: dependent.Key, Provider: provider.Key, Reason: reason}
}

func keysOf(decls []entities.Decl) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(decls))
	for _, item := range decls {
		out = append(out, item.Key)
	}

	return out
}
