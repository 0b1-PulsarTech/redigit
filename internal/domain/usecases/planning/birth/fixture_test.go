package birth

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func decl(file, name string, unit entities.UnitID) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit: unit,
	}
}

func needs(dependent, provider entities.Decl) entities.Edge {
	return entities.Edge{
		Dependent: dependent.Key,
		Provider:  provider.Key,
		Reason:    entities.ReasonReference,
	}
}

func testDecl(file, name string, unit entities.UnitID) entities.Decl {
	out := decl(file, name, unit)
	out.IsTest = true

	return out
}
