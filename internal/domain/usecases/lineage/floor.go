package lineage

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func Floor(result Result) map[entities.DeclKey]entities.Position {
	floor := make(map[entities.DeclKey]entities.Position, len(result.Births))

	for key, birth := range result.Births {
		// A renamed declaration's At is already its predecessor's: no walk through Via.
		floor[key] = birth.At
	}

	return floor
}
