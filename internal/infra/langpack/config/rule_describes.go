package config

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// describesRule births a document with the first code it sits beside: a
// package's README. The code waits for the document, so the two land together.
// A document that only names code is namedRule's.
type describesRule struct{}

func (describesRule) Name() string { return "describes" }

func (describesRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	var edges []entities.Edge

	forEachDescribed(snap, decls, func(doc, code entities.DeclKey) {
		edges = append(edges, entities.Edge{
			Dependent: code,
			Provider:  doc,
			Reason:    entities.ReasonDescribes,
		})
	})

	return edges, nil
}

// Anchors lower the document to its code: it is first touched whenever the
// author wrote it, and the fixed point only raises the code to wait for it.
func (describesRule) Anchors(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	var anchors []entities.Anchor

	forEachDescribed(snap, decls, func(doc, code entities.DeclKey) {
		anchors = append(anchors, entities.Anchor{
			Decl:   doc,
			To:     code,
			Reason: entities.ReasonDescribes,
		})
	})

	return anchors, nil
}

func forEachDescribed(
	snap entities.Snapshot,
	decls []entities.Decl,
	visit func(doc, code entities.DeclKey),
) {
	placed := classify(snap, decls)

	for _, decl := range decls {
		if placed[decl.Key].role != roleProse {
			continue
		}

		for _, code := range placed[decl.Key].homes {
			visit(decl.Key, code)
		}
	}
}
