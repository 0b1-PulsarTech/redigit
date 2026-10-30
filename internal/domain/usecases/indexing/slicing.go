package indexing

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// wiringCut is the functions that assemble the program, cut into statements.
type wiringCut struct {
	decls []entities.Decl
	// replaced is the functions that were cut: what they read is asked again,
	// because the first answer attributed every reader to the whole function.
	replaced map[entities.DeclKey]bool
	// added is the heads and statements that now stand where they were.
	added map[entities.DeclKey]bool
	// body is what the statements need from their own function.
	body []entities.Edge
}

// cutWiring slices every function the graph reads as wiring, for the languages
// that can. A route table, an injector or a bootstrap list is written one
// registration at a time, each with the feature it registers; read as a single
// declaration it waits for the last feature and the program's entry lands at
// the end of the history, so it is cut into the statements it is made of.
func (i Indexer) cutWiring(
	ctx context.Context,
	graph entities.Graph,
	decls []entities.Decl,
	where locator,
) (wiringCut, error) {
	roots := entities.FindWiring(graph)
	byFile := map[string][]entities.Decl{}

	for _, decl := range decls {
		byFile[decl.Key.File] = append(byFile[decl.Key.File], decl)
	}

	// Map order is random and a stable sort keeps equal keys in input order, so fix the input.
	files := slices.Sorted(maps.Keys(byFile))
	cut := wiringCut{replaced: map[entities.DeclKey]bool{}, added: map[entities.DeclKey]bool{}}

	for _, file := range files {
		if err := i.cutFile(ctx, file, byFile, roots, &cut, where); err != nil {
			return wiringCut{}, err
		}
	}

	for _, file := range files {
		cut.decls = append(cut.decls, byFile[file]...)
	}

	slices.SortStableFunc(cut.decls, compareDeclKeys)

	return cut, nil
}

func (i Indexer) cutFile(
	ctx context.Context,
	file string,
	byFile map[string][]entities.Decl,
	roots map[entities.DeclKey]bool,
	cut *wiringCut,
	where locator,
) error {
	lang, owned := i.languageFor(file)
	slicer, canSlice := lang.Syntax.(Slicer)

	if !owned || !canSlice {
		return nil
	}

	var candidates []entities.Decl

	for _, decl := range byFile[file] {
		if roots[decl.Key] && isCuttable(decl) {
			candidates = append(candidates, decl)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	source, err := i.tree.Read(ctx, file)
	if err != nil {
		return fmt.Errorf("read %s to slice: %w", file, err)
	}

	pieces := map[entities.DeclKey]Sliced{}

	for _, fn := range candidates {
		sliced, cuttable, err := slicer.Slice(ctx, fn, source)
		if err != nil {
			return fmt.Errorf("slice %s: %w", fn.Key, err)
		}

		if cuttable {
			pieces[fn.Key] = sliced
		}
	}

	if len(pieces) == 0 {
		return nil
	}

	var rewritten []entities.Decl

	for _, decl := range byFile[file] {
		sliced, wasCut := pieces[decl.Key]
		if !wasCut {
			rewritten = append(rewritten, decl)

			continue
		}

		cut.replaced[decl.Key] = true
		cut.body = append(cut.body, sliced.Edges...)
		rewritten = append(rewritten, sliced.Head)
		rewritten = append(rewritten, sliced.Members...)

		cut.added[sliced.Head.Key] = true

		for _, member := range sliced.Members {
			cut.added[member.Key] = true
		}
	}

	measure(file, source, rewritten)
	where.add(file, source, rewritten)

	byFile[file] = rewritten

	return nil
}

func isCuttable(decl entities.Decl) bool {
	kind := decl.Key.Kind

	return (kind == entities.KindFunc || kind == entities.KindMethod) &&
		!entities.IsTestSide(decl) && !decl.Generated
}

func compareDeclKeys(a, b entities.Decl) int { return entities.CompareKeys(a.Key, b.Key) }

// reattach asks again who reads what the cut functions read, and keeps the
// answers that land on a head or a statement. The locator now tells the
// statements apart, so each reader is attributed to the one that wrote it.
func (i Indexer) reattach(
	ctx context.Context,
	graph entities.Graph,
	cut wiringCut,
	where locator,
) []entities.Edge {
	providers := map[entities.DeclKey]bool{}

	for key := range cut.replaced {
		for _, edge := range graph.Needs(key) {
			if edge.Reason == entities.ReasonReference || edge.Reason == entities.ReasonImplements {
				providers[edge.Provider] = true
			}
		}
	}

	interfaces := map[entities.DeclKey]bool{}

	for _, decl := range graph.Decls() {
		if decl.IsInterface {
			interfaces[decl.Key] = true
		}
	}

	var out []entities.Edge

	for _, key := range sortedKeySet(providers) {
		provider, found := graph.Decl(key)
		if !found {
			continue
		}

		edges, _ := i.edgesOf(ctx, provider, where, interfaces)

		for _, edge := range edges {
			if cut.added[edge.Dependent] {
				out = append(out, edge)
			}
		}
	}

	return out
}

func sortedKeySet(set map[entities.DeclKey]bool) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(set))
	for key := range set {
		out = append(out, key)
	}

	slices.SortFunc(out, entities.CompareKeys)

	return out
}
