package indexing

import (
	"context"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// assemble turns the declarations and the edges a resolver proved into a graph,
// adding what the rules of each language see that a resolver cannot.
func (i Indexer) assemble(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
	edges []entities.Edge,
) (entities.Graph, error) {
	ruleEdges, anchors, err := i.applyRules(ctx, snap, decls)
	if err != nil {
		return entities.Graph{}, err
	}

	if err = i.applyAtoms(ctx, snap, decls); err != nil {
		return entities.Graph{}, err
	}

	return entities.NewGraph(
		decls, i.filterEdges(snap, append(slices.Clone(edges), ruleEdges...)), anchors,
	), nil
}

// assembleCut builds the graph again over the cut functions: what the whole
// function read is dropped, and each statement gets back the readers it wrote.
func (i Indexer) assembleCut(
	ctx context.Context,
	snap entities.Snapshot,
	first entities.Graph,
	edges []entities.Edge,
	cut wiringCut,
	where locator,
) (entities.Graph, error) {
	again := i.reattach(ctx, first, cut, where)
	kept := make([]entities.Edge, 0, len(edges)+len(again)+len(cut.body))

	for _, edge := range edges {
		if !cut.replaced[edge.Dependent] {
			kept = append(kept, edge)
		}
	}

	kept = append(kept, again...)
	kept = append(kept, cut.body...)

	slices.SortFunc(kept, compareEdges)

	return i.assemble(ctx, snap, cut.decls, slices.Compact(kept))
}

func (i Indexer) applyRules(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, []entities.Anchor, error) {
	var (
		edges   []entities.Edge
		anchors []entities.Anchor
	)

	for _, lang := range i.langs {
		if lang.Rules == nil {
			continue
		}

		found, err := lang.Rules.Edges(ctx, snap, decls)
		if err != nil {
			return nil, nil, fmt.Errorf("apply %s rules: %w", lang.Name, err)
		}

		pulled, err := lang.Rules.Anchors(ctx, snap, decls)
		if err != nil {
			return nil, nil, fmt.Errorf("apply %s anchors: %w", lang.Name, err)
		}

		edges = append(edges, found...)
		anchors = append(anchors, pulled...)
	}

	return edges, anchors, nil
}

// filterEdges lets each language's rules drop the edges they know to be noise.
func (i Indexer) filterEdges(snap entities.Snapshot, edges []entities.Edge) []entities.Edge {
	for _, lang := range i.langs {
		filter, isFilter := lang.Rules.(EdgeFilter)
		if isFilter {
			edges = filter.FilterEdges(snap, edges)
		}
	}

	return edges
}

// applyAtoms lets each language's rules group declarations that must be born
// together. The decls are the slice the graph is built from, so the group is
// written onto them in place.
func (i Indexer) applyAtoms(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) error {
	for _, lang := range i.langs {
		grouping, isGrouping := lang.Rules.(Atomizer)
		if !isGrouping {
			continue
		}

		atoms, err := grouping.Atoms(ctx, snap, decls)
		if err != nil {
			return fmt.Errorf("apply %s atoms: %w", lang.Name, err)
		}

		absorbAtoms(decls, atoms)
	}

	return nil
}

// resolvable keeps the declarations a language server can answer about.
func resolvable(indexer Indexer, decls []entities.Decl) []entities.Decl {
	out := make([]entities.Decl, 0, len(decls))

	for _, decl := range decls {
		if indexer.resolvedFile(decl.Key.File) {
			out = append(out, decl)
		}
	}

	return out
}

// absorbAtoms writes the groups a rule named onto the declarations. One that
// already belongs to an atom brings the whole atom along: the two must be born
// together, and leaving the old members behind would split the group the rule
// asked for (a go.mod header and its go.work line are an atom of their own).
func absorbAtoms(decls []entities.Decl, atoms map[entities.DeclKey]string) {
	renamed := map[string]string{}

	for _, decl := range decls {
		if name, grouped := atoms[decl.Key]; grouped && decl.Atom != "" && decl.Atom != name {
			if _, taken := renamed[decl.Atom]; !taken {
				renamed[decl.Atom] = name
			}
		}
	}

	for index := range decls {
		if name, absorbed := renamed[decls[index].Atom]; absorbed {
			decls[index].Atom = name

			continue
		}

		if name, grouped := atoms[decls[index].Key]; grouped && decls[index].Atom == "" {
			decls[index].Atom = name
		}
	}
}
