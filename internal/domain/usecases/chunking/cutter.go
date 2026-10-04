package chunking

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Piece is one chunk of one file, and the position of the published history
// that first held it.
type Piece struct {
	Decl    entities.Decl
	Chunker string

	// BornAt comes from cutting each commit's own version of the file and
	// looking for the key, never from matching the name against the commit's
	// text: a name occurs in comments, and where the consumer declares its
	// ports the same name is declared on purpose in several packages, so an
	// occurrence is not a presence.
	BornAt  entities.Position
	BornRev string
}

// Cutter cuts every file of a snapshot into pieces and dates each piece
// against the published history.
type Cutter struct {
	source    Source
	revisions Revisions
	chunkers  []Chunker
}

// NewCutter tries chunkers in order and gives the first one that claims a path
// the whole file. Order is the caller's: the last entry usually claims
// everything, so a more specific chunker must come before it.
func NewCutter(source Source, revisions Revisions, chunkers []Chunker) Cutter {
	return Cutter{source: source, revisions: revisions, chunkers: chunkers}
}

// Pieces cuts every file of snap and dates each piece. A file no chunker
// claims contributes nothing rather than a whole-file piece: whether an
// unclaimed file deserves one is the caller's choice, made by what it puts in
// the chunker list.
func (c Cutter) Pieces(ctx context.Context, snap entities.Snapshot) ([]Piece, error) {
	files := slices.Clone(snap.Files)
	slices.SortFunc(files, cmp.Compare)

	var pieces []Piece

	for _, path := range files {
		chunker := c.pick(path)
		if chunker == nil {
			continue
		}

		filePieces, err := c.cut(ctx, snap, chunker, path)
		if err != nil {
			return nil, err
		}

		pieces = append(pieces, filePieces...)
	}

	return pieces, nil
}

//nolint:ireturn // the interface is what the registry holds.
func (c Cutter) pick(path string) Chunker {
	for _, chunker := range c.chunkers {
		if chunker.Handles(path) {
			return chunker
		}
	}

	return nil
}

// cut chunks path's final content, then dates each piece against every commit
// that touched the path.
func (c Cutter) cut(
	ctx context.Context,
	snap entities.Snapshot,
	chunker Chunker,
	path string,
) ([]Piece, error) {
	content, err := c.source.Read(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	decls, err := chunker.Chunks(ctx, path, content)
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", path, err)
	}

	births, err := c.births(ctx, snap, chunker, path)
	if err != nil {
		return nil, err
	}

	pieces := make([]Piece, 0, len(decls))

	for _, decl := range decls {
		// Comma-ok, not a bare lookup: a missing key yields the zero birth,
		// whose Position is 0 - a real position, and the earliest one. A piece
		// the published history never held would claim to be born first.
		found, ok := births[decl.Key]
		if !ok {
			found = birth{at: entities.NoPosition}
		}

		pieces = append(pieces, Piece{
			Decl:    decl,
			Chunker: chunker.Name(),
			BornAt:  found.at,
			BornRev: found.rev,
		})
	}

	return pieces, nil
}

// birth is where one piece first appeared.
type birth struct {
	at  entities.Position
	rev string
}

// births walks the commits that touched path, in published order, cuts each
// version, and records the first whose cutting already held each key.
func (c Cutter) births(
	ctx context.Context,
	snap entities.Snapshot,
	chunker Chunker,
	path string,
) (map[entities.DeclKey]birth, error) {
	found := map[entities.DeclKey]birth{}

	touches := touchesOf(snap, path)
	slices.SortFunc(touches, func(x, y entities.SourceTouch) int {
		return cmp.Compare(x.Position, y.Position)
	})

	for _, touch := range touches {
		content, err := c.revisions.ReadAt(ctx, touch.Rev, path)
		// A file deleted and later restored is still touched by the commit
		// that removed it, and that commit holds no version to cut. Absent is
		// not a failure.
		if err != nil {
			continue
		}

		decls, chunkErr := chunker.Chunks(ctx, path, content)
		if chunkErr != nil {
			return nil, fmt.Errorf("chunk %s at %s: %w", path, touch.Rev, chunkErr)
		}

		for _, decl := range decls {
			if _, seen := found[decl.Key]; !seen {
				found[decl.Key] = birth{at: touch.Position, rev: touch.Rev}
			}
		}
	}

	return found, nil
}

func touchesOf(snap entities.Snapshot, path string) []entities.SourceTouch {
	var touches []entities.SourceTouch

	for _, touch := range snap.History {
		if slices.Contains(touch.Touched, path) {
			touches = append(touches, touch)
		}
	}

	return touches
}
