package deferral

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func decl(file, name string, unit entities.UnitID) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit: unit,
	}
}

func testDecl(file, name string, unit entities.UnitID) entities.Decl {
	out := decl(file, name, unit)
	out.IsTest = true

	return out
}

func needs(dependent, provider entities.Decl) entities.Edge {
	return entities.Edge{
		Dependent: dependent.Key,
		Provider:  provider.Key,
		Reason:    entities.ReasonReference,
	}
}

func sharedTestOwnership(decls []entities.Decl, owned ...entities.Decl) owners.Ownership {
	own := owners.Ownership{
		Owner:    map[entities.DeclKey]reach.FeatureID{},
		Deferred: reach.Wiring{},
		Roots:    reach.Wiring{},
		Reach:    map[entities.DeclKey]int{},
	}

	for _, member := range decls {
		own.Owner[member.Key] = reach.SharedFeature

		if member.IsTest || strings.HasSuffix(member.Key.File, "_test.go") {
			own.Deferred[member.Key] = true
		} else {
			own.Reach[member.Key] = 1
		}
	}

	for _, member := range owned {
		own.Owner[member.Key] = "a"
	}

	return own
}

func readyAfter(
	decls []entities.Decl,
	edges []entities.Edge,
	built []entities.DeclKey,
	fresh entities.Decl,
	owned ...entities.Decl,
) []entities.DeclKey {
	born := map[entities.DeclKey]bool{}
	for _, key := range built {
		born[key] = true
	}

	return ReadyDeferred(
		entities.NewGraph(decls, edges, nil),
		sharedTestOwnership(decls, append([]entities.Decl{fresh}, owned...)...),
		born,
		map[entities.DeclKey]bool{fresh.Key: true},
	)
}
