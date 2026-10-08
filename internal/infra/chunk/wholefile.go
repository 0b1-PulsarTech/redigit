package chunk

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// fileDeclName names a Decl standing for a whole file rather than a piece of
// one. It matches internal/infra/langpack/golang/decl.go's convention for a
// Go file that declares nothing, and is not a legal identifier or module
// path, so it can never collide with a real declaration name.
const fileDeclName = "<file>"

// WholeFile is the chunker of last resort: one Decl, spanning the whole
// file, for any path no other chunker claims. A file with no reader for its
// format is still a file the final tree needs, so it must become a citizen
// of the ordering rather than vanish from it.
//
// A hand-written text file over MaxLines becomes one Decl per MaxLines lines:
// a 995-line changelog passes the commit cap alone, and no other boundary
// exists in a format redigit does not read. Generated, binary and MaxLines 0
// split nothing.
type WholeFile struct {
	MaxLines int
}

var _ indexing.Chunker = WholeFile{}

func (WholeFile) Name() string { return "whole-file" }

func (WholeFile) Handles(string) bool { return true }

func (w WholeFile) Chunks(
	_ context.Context,
	path string,
	source []byte,
) ([]entities.Decl, error) {
	if w.MaxLines < 1 || bytes.IndexByte(source, 0) >= 0 || indexing.IsGenerated(path, source) {
		return []entities.Decl{wholeFileDecl(path, source)}, nil
	}

	return linePieces(path, source, w.MaxLines), nil
}

// linePieces cuts source into runs of maxLines lines. The first keeps the
// whole-file name, so a file that fits stays what it always was.
func linePieces(path string, source []byte, maxLines int) []entities.Decl {
	lines := splitLines(source)
	if len(lines) <= maxLines {
		return []entities.Decl{wholeFileDecl(path, source)}
	}

	var pieces []entities.Decl

	for first := 0; first < len(lines); first += maxLines {
		last := min(first+maxLines, len(lines)) - 1

		name := fileDeclName
		if first > 0 {
			name = fmt.Sprintf("%s#%d", fileDeclName, first/maxLines+1)
		}

		pieces = append(pieces, entities.Decl{
			Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: name},
			Unit:     entities.UnitID(filepath.Dir(path)),
			Span:     entities.Span{Start: lines[first].Start, End: lines[last].End},
			Selector: entities.Location{File: path, Line: first},
		})
	}

	return pieces
}

// wholeFileDecl builds the single Decl a whole, unsplit file becomes: used
// here for any unclaimed path, and reused by SQLQueries for a ".sql" file
// under a queries directory that holds no "-- name:" block at all.
func wholeFileDecl(path string, source []byte) entities.Decl {
	return entities.Decl{
		Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: fileDeclName},
		Unit:     entities.UnitID(filepath.Dir(path)),
		Span:     entities.Span{Start: 0, End: len(source)},
		Selector: entities.Location{File: path},
	}
}
