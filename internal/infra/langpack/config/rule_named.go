package config

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// namedRule makes a document wait for the code its text names. A specification
// opened the history because it was born with the first code under a directory
// it names, and 20 of the first 71 commits were documents about a program that
// did not exist yet. A document that names nothing is left for the end.
type namedRule struct{}

func (namedRule) Name() string { return "named" }

func (namedRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	var edges []entities.Edge

	placed := classify(snap, decls)

	for _, decl := range decls {
		if placed[decl.Key].role != roleNamed {
			continue
		}

		for _, code := range placed[decl.Key].homes {
			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  code,
				Reason:    entities.ReasonProse,
			})
		}
	}

	return edges, nil
}

func (namedRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
