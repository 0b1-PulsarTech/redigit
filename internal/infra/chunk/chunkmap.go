package chunk

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// ErrOverlappingSpans is returned when two entries of a ChunkMap intersect. A
// wrong map is worse than no map, so this is refused outright rather than
// silently merged.
var ErrOverlappingSpans = errors.New("chunk map spans overlap")

// ErrSpanOutOfRange is returned when a ChunkMap entry's span falls outside
// the file it names.
var ErrSpanOutOfRange = errors.New("chunk map span out of range")

// MapApplier is indexing.Chunker for the chunking an operator supplies from
// outside, for a format this package has no reader for. It guarantees the
// two things the design promises: no two spans in the map are allowed to
// overlap, and any byte range the map leaves uncovered becomes a Decl of its
// own, so nothing disappears because a span forgot it.
type MapApplier struct {
	Map entities.ChunkMap
}

var _ indexing.Chunker = MapApplier{}

func (MapApplier) Name() string { return "chunk-map" }

func (a MapApplier) Handles(path string) bool {
	_, ok := a.Map[path]

	return ok
}

func (a MapApplier) Chunks(
	_ context.Context,
	path string,
	source []byte,
) ([]entities.Decl, error) {
	spans := slices.Clone(a.Map[path])
	slices.SortFunc(spans, func(x, y entities.ChunkSpan) int { return x.Span.Start - y.Span.Start })

	if err := validateSpans(path, source, spans); err != nil {
		return nil, err
	}

	unit := entities.UnitID(filepath.Dir(path))

	var decls []entities.Decl

	cursor := 0
	for _, span := range spans {
		if span.Span.Start > cursor {
			decls = append(decls, gapDecl(path, unit, source, cursor, span.Span.Start))
		}

		decls = append(decls, entities.Decl{
			Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: span.Name},
			Unit:     unit,
			Span:     span.Span,
			Selector: entities.Location{File: path},
		})
		cursor = span.Span.End
	}

	if cursor < len(source) {
		decls = append(decls, gapDecl(path, unit, source, cursor, len(source)))
	}

	return decls, nil
}

// validateSpans checks the two guarantees a ChunkMap must satisfy before any
// Decl is built from it: every span fits inside the file, and no two spans
// overlap. Spans must already be sorted by Start.
func validateSpans(path string, source []byte, spans []entities.ChunkSpan) error {
	for _, span := range spans {
		if span.Span.Start < 0 || span.Span.End > len(source) || span.Span.Start > span.Span.End {
			return fmt.Errorf("%w: %q %v in %s", ErrSpanOutOfRange, span.Name, span.Span, path)
		}
	}

	for i := 1; i < len(spans); i++ {
		prev, cur := spans[i-1], spans[i]
		if cur.Span.Start < prev.Span.End {
			return fmt.Errorf(
				"%w: %q %v and %q %v in %s",
				ErrOverlappingSpans, prev.Name, prev.Span, cur.Name, cur.Span, path,
			)
		}
	}

	return nil
}

// gapDecl builds the residual Decl for a byte range the ChunkMap left
// uncovered. Its name embeds the range because two gaps in the same file
// would otherwise share the same DeclKey.
func gapDecl(path string, unit entities.UnitID, source []byte, start, end int) entities.Decl {
	return entities.Decl{
		Key: entities.DeclKey{
			File: path,
			Kind: entities.KindOther,
			Name: fmt.Sprintf("<gap:%d-%d>", start, end),
		},
		Unit:     unit,
		Span:     entities.Span{Start: start, End: end},
		Selector: locationAt(path, source, splitLines(source), start),
	}
}
