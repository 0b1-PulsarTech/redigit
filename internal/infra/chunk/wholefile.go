package chunk

import (
	"context"
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
type WholeFile struct{}

var _ indexing.Chunker = WholeFile{}

func (WholeFile) Name() string { return "whole-file" }

func (WholeFile) Handles(string) bool { return true }

func (WholeFile) Chunks(
	_ context.Context,
	path string,
	source []byte,
) ([]entities.Decl, error) {
	return []entities.Decl{wholeFileDecl(path, source)}, nil
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
