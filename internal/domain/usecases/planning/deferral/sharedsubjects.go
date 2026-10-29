package deferral

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func sharedSubjects(
	graph entities.Graph,
	own owners.Ownership,
) map[entities.DeclKey][]entities.DeclKey {
	out := map[entities.DeclKey][]entities.DeclKey{}

	for test, subjects := range reach.TestSubjects(graph) {
		for _, subject := range subjects {
			if own.Owner[subject] == reach.SharedFeature {
				out[test] = append(out[test], subject)
			}
		}
	}

	return out
}
