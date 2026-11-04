package indexing

import (
	"cmp"
	"context"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// fileSpans holds one file's declarations ordered by where they start, so a
// location reported by a resolver can be read back as the declaration that
// encloses it.
type fileSpans struct {
	lines lineIndex
	spans []spanOf
}

type spanOf struct {
	span entities.Span
	key  entities.DeclKey
}

// locator answers which declaration encloses a location. Without it a
// reference is only a coordinate, and the graph needs to know which
// declaration did the reading.
type locator struct {
	byFile map[string]*fileSpans
}

func newLocator() locator {
	return locator{byFile: map[string]*fileSpans{}}
}

// Enclosing reports the declaration containing a location, and whether one
// does. A location in a file's import block or package clause belongs to no
// declaration, which is a real answer rather than a failure.
func (l locator) Enclosing(at entities.Location) (entities.DeclKey, bool) {
	entry, known := l.byFile[at.File]
	if !known {
		return entities.DeclKey{}, false
	}

	offset := entry.lines.ByteOffset(at.Line, at.Char)
	if offset < 0 {
		return entities.DeclKey{}, false
	}

	// The last declaration starting at or before the offset is the only one
	// that can contain it, because spans of top-level declarations do not nest.
	found, exact := slices.BinarySearchFunc(
		entry.spans,
		offset,
		func(candidate spanOf, target int) int { return cmp.Compare(candidate.span.Start, target) },
	)

	if !exact {
		found--
	}

	if found < 0 || found >= len(entry.spans) {
		return entities.DeclKey{}, false
	}

	span := entry.spans[found]
	if offset >= span.span.End {
		return entities.DeclKey{}, false
	}

	return span.key, true
}

func (l locator) add(path string, source []byte, decls []entities.Decl) {
	entry := &fileSpans{lines: newLineIndex(source), spans: make([]spanOf, 0, len(decls))}

	for _, decl := range decls {
		entry.spans = append(entry.spans, spanOf{span: decl.Span, key: decl.Key})
	}

	slices.SortFunc(entry.spans, func(a, b spanOf) int {
		return cmp.Compare(a.span.Start, b.span.Start)
	})

	l.byFile[path] = entry
}

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
