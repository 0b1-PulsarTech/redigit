package anchoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const (
	reasonRenamed  = "renamed after the anchor"
	reasonGone     = "absent from the final tree"
	reasonReverted = "back to its pre-image"
	reasonUnparsed = "a revision of the file does not parse"
)

type (
	entry struct {
		key      entities.DeclKey
		span     entities.Span
		preimage string
	}

	skip struct {
		key    entities.DeclKey
		reason string
	}

	delta struct {
		entries []entry
		skipped []skip
		born    []entities.DeclKey
	}

	finalView struct {
		graph  entities.Graph
		anchor Anchor
		sliced map[sliceKey]bool
		texts  map[string][]byte
	}
)

func (planner Planner) delta(
	ctx context.Context,
	graph entities.Graph,
	anchor Anchor,
	revision Revision,
) (delta, error) {
	var out delta

	view := finalView{
		graph: graph, anchor: anchor, sliced: slicedFuncs(graph), texts: map[string][]byte{},
	}

	for _, change := range revision.Files {
		part, err := planner.fileDelta(ctx, view, change)
		if err != nil {
			return delta{}, err
		}

		out.entries = append(out.entries, part.entries...)
		out.skipped = append(out.skipped, part.skipped...)
		out.born = append(out.born, part.born...)
	}

	return out, nil
}

func (planner Planner) fileDelta(
	ctx context.Context,
	view finalView,
	change FileChange,
) (delta, error) {
	language, isOwned := planner.languageFor(change.Path)
	if !isOwned || change.Blob == "" {
		return delta{}, nil
	}

	found, born, err := planner.changedDecls(ctx, language.Parser, change)
	if errors.Is(err, errUnparsed) {
		skipped := skip{key: entities.DeclKey{File: change.Path}, reason: reasonUnparsed}

		return delta{skipped: []skip{skipped}}, nil
	}

	if err != nil {
		return delta{}, err
	}

	var out delta

	for _, key := range born {
		final := planner.cfg.Renames.Final(key, view.anchor.Position)
		if _, isPresent := view.graph.Decl(final); !isPresent {
			planner.reportMissing(view.anchor.Rev, final)

			continue
		}

		out.born = append(out.born, final)
	}

	for _, one := range found {
		final := planner.cfg.Renames.Final(one.atAnchor, view.anchor.Position)

		var (
			placed entry
			reason string
		)

		if placed, reason, err = planner.resolve(ctx, view, final, one); err != nil {
			return delta{}, err
		}

		// A move to another file keeps the declaration; only a new name or kind loses it.
		if reason == "" && (final.Kind != one.atAnchor.Kind || final.Name != one.atAnchor.Name) {
			reason = reasonRenamed
		}

		if reason != "" {
			out.skipped = append(out.skipped, skip{key: final, reason: reason})

			continue
		}

		out.entries = append(out.entries, placed)
	}

	return out, nil
}

func (planner Planner) resolve(
	ctx context.Context,
	view finalView,
	final entities.DeclKey,
	one changed,
) (entry, string, error) {
	decl, isFound := view.graph.Decl(final)
	if !isFound {
		return entry{}, reasonGone, nil
	}

	if view.sliced[sliceKey{final.File, final.Name}] {
		return entry{}, "cut into statements", nil
	}

	text, isCached := view.texts[final.File]
	if !isCached {
		var err error

		if text, err = planner.cfg.Finals.Read(ctx, final.File); err != nil {
			return entry{}, "", fmt.Errorf("read final %s: %w", final.File, err)
		}

		view.texts[final.File] = text
	}

	if decl.Span.End > len(text) || decl.Span.Start > decl.Span.End {
		return entry{}, "", fmt.Errorf("%w: %s spans past its final file", ErrAnchor, final)
	}

	if squeezed(string(text[decl.Span.Start:decl.Span.End])) == squeezed(one.preimage) {
		return entry{}, reasonReverted, nil
	}

	return entry{key: final, span: decl.Span, preimage: one.preimage}, "", nil
}
