package config

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// describesRule births a document with the first code it describes. A guide to
// a package, a spec naming the directories it covers, a README beside the code:
// none has a reader, so all were left for the trailing commit, and docs/ was
// born in the last decile of a 1208-commit history.
//
// The code waits for the document, not the other way: the document is the
// provider, so it lands no later than the earliest of the code it names, in
// whichever commit the planner cuts that code. What a document names is read
// from its text by path (see codeIndex); one naming nothing is scaffold, never
// the end.
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
		for _, code := range placed[decl.Key].homes {
			visit(decl.Key, code)
		}
	}
}
