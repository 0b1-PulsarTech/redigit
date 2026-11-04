package indexing

import (
	"context"
	"fmt"
	"maps"
	"path"
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

// cutWiring slices every function the graph reads as wiring, and every one a
// slice.functions glob names, for the languages that can. A route table, an
// injector or a bootstrap list is written one registration at a time, each with
// the feature it registers; read as a single declaration it waits for the last
// feature and the program's entry lands at the end of the history, so it is cut
// into the statements it is made of.
func (i Indexer) cutWiring(
	ctx context.Context,
	graph entities.Graph,
	decls []entities.Decl,
	where locator,
) (wiringCut, error) {
	roots := entities.FindWiring(graph)
	i.selectFunctions(decls, roots)

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

	// A silently ignored setting is one the author believes is working.
	if err := i.refuseDeadGlobs(cut.replaced); err != nil {
		return wiringCut{}, err
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

func globMatches(glob, name string) bool {
	hit, _ := path.Match(glob, name)

	return hit
}

func (i Indexer) selectFunctions(decls []entities.Decl, roots map[entities.DeclKey]bool) {
	for _, lang := range i.langs {
		selector, isSelector := lang.Syntax.(Selector)
		if !isSelector {
			continue
		}

		for _, decl := range decls {
			if lang.Owns(decl.Key.File) && isCuttable(decl) &&
				slices.ContainsFunc(selector.Functions(), func(glob string) bool {
					return globMatches(glob, decl.Key.Name)
				}) {
				roots[decl.Key] = true
			}
		}
	}
}
