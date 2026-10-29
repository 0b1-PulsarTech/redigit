package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func edgeOf(dependent, provider entities.Decl, reason entities.EdgeReason) entities.Edge {
	return entities.Edge{Dependent: dependent.Key, Provider: provider.Key, Reason: reason}
}
