package sequence

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

func twoFeatures() (entities.Graph, entities.Ordering) {
	kernel := decl("kernel/id.go", "NewID", "kernel")

	leadUse := decl("lead/usecases/create.go", "Create", "lead/usecases")
	leadRepo := decl("lead/repo/lead.go", "Insert", "lead/repo")
	leadAPI := decl("lead/api/handler.go", "Handle", "lead/api")

	billUse := decl("bill/usecases/charge.go", "Charge", "bill/usecases")
	billAPI := decl("bill/api/handler.go", "Handle", "bill/api")

	decls := []entities.Decl{kernel, leadUse, leadRepo, leadAPI, billUse, billAPI}
	edges := []entities.Edge{
		needs(leadAPI, leadUse),
		needs(leadUse, leadRepo),
		needs(leadRepo, kernel),
		needs(billAPI, billUse),
		needs(billUse, kernel),
	}

	at := map[entities.DeclKey]entities.Position{
		kernel.Key:   0,
		leadRepo.Key: 1,
		leadUse.Key:  2,
		leadAPI.Key:  3,
		billUse.Key:  4,
		billAPI.Key:  5,
	}

	return entities.NewGraph(decls, edges, nil), entities.Ordering{At: at}
}
