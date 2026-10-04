package chunking

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// lineChunker cuts a file into one piece per non-empty line, named by the
// line's text. It stands in for a real format: what every test below asserts
// is when a piece was born, not how a particular syntax is split.
type lineChunker struct{}

var _ Chunker = lineChunker{}

func (lineChunker) Name() string { return "line" }

func (lineChunker) Handles(path string) bool { return strings.HasSuffix(path, ".txt") }

func (lineChunker) Chunks(
	_ context.Context,
	path string,
	source []byte,
) ([]entities.Decl, error) {
	var (
		decls  []entities.Decl
		cursor int
	)

	for _, line := range strings.SplitAfter(string(source), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			decls = append(decls, entities.Decl{
				Key:  entities.DeclKey{File: path, Kind: entities.KindOther, Name: trimmed},
				Unit: entities.UnitID(path),
				Span: entities.Span{Start: cursor, End: cursor + len(line)},
			})
		}

		cursor += len(line)
	}

	return decls, nil
}

func snapshotOf(revs ...string) entities.Snapshot {
	const path = "queries.txt"

	history := make([]entities.SourceTouch, 0, len(revs))
	for index, rev := range revs {
		history = append(history, entities.SourceTouch{
			Position: entities.Position(index),
			Rev:      rev,
			Touched:  []string{path},
		})
	}

	return entities.Snapshot{Files: []string{path}, History: history}
}

func byName(pieces []Piece) map[string]Piece {
	out := make(map[string]Piece, len(pieces))
	for _, piece := range pieces {
		out[piece.Decl.Key.Name] = piece
	}

	return out
}

func TestCutterDatesEachPieceAtItsOwnFirstCommit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	const path = "queries.txt"

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), path).Return([]byte("one\ntwo\nthree\n"), nil)

	revisions := NewMockRevisions(ctrl)
	revisions.EXPECT().ReadAt(gomock.Any(), "r0", path).Return([]byte("one\n"), nil)
	revisions.EXPECT().ReadAt(gomock.Any(), "r1", path).Return([]byte("one\ntwo\n"), nil)
	revisions.EXPECT().ReadAt(gomock.Any(), "r2", path).Return([]byte("one\ntwo\nthree\n"), nil)

	cutter := NewCutter(source, revisions, []Chunker{lineChunker{}})

	pieces, err := cutter.Pieces(t.Context(), snapshotOf("r0", "r1", "r2"))
	if err != nil {
		t.Fatalf("Pieces() = %v", err)
	}

	// The whole point: the file is touched by three commits, and the three
	// pieces are born at three different ones. A file-shaped answer would date
	// all three at position 0.
	testCases := []struct {
		name    string
		wantAt  entities.Position
		wantRev string
	}{
		{name: "one", wantAt: 0, wantRev: "r0"},
		{name: "two", wantAt: 1, wantRev: "r1"},
		{name: "three", wantAt: 2, wantRev: "r2"},
	}

	got := byName(pieces)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			piece, ok := got[testCase.name]
			if !ok {
				t.Fatalf("piece %q is missing from %d pieces", testCase.name, len(pieces))
			}

			if piece.BornAt != testCase.wantAt {
				t.Fatalf("BornAt = %d, want %d", piece.BornAt, testCase.wantAt)
			}

			if piece.BornRev != testCase.wantRev {
				t.Fatalf("BornRev = %q, want %q", piece.BornRev, testCase.wantRev)
			}
		})
	}
}

// A piece no commit of the published history ever held must report
// NoPosition. Reporting 0 would place it at the earliest position there is,
// which is the most damaging wrong answer available.
func TestCutterReportsNoPositionForAPieceTheHistoryNeverHeld(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	const path = "queries.txt"

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), path).Return([]byte("kept\nadded later\n"), nil)

	revisions := NewMockRevisions(ctrl)
	revisions.EXPECT().ReadAt(gomock.Any(), "r0", path).Return([]byte("kept\n"), nil)

	cutter := NewCutter(source, revisions, []Chunker{lineChunker{}})

	pieces, err := cutter.Pieces(t.Context(), snapshotOf("r0"))
	if err != nil {
		t.Fatalf("Pieces() = %v", err)
	}

	got := byName(pieces)

	if born := got["kept"].BornAt; born != 0 {
		t.Fatalf("kept BornAt = %d, want 0", born)
	}

	orphan := got["added later"]
	if orphan.BornAt != entities.NoPosition {
		t.Fatalf("BornAt = %d, want NoPosition (%d)", orphan.BornAt, entities.NoPosition)
	}

	if orphan.BornRev != "" {
		t.Fatalf("BornRev = %q, want empty", orphan.BornRev)
	}
}

// A commit that deletes a path still touches it, and holds no version to
// chunk. That must not fail the run, and must not hide the pieces the later
// version restores.
func TestCutterSkipsARevisionThePathIsAbsentFrom(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	const path = "queries.txt"

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), path).Return([]byte("one\n"), nil)

	revisions := NewMockRevisions(ctrl)
	revisions.EXPECT().
		ReadAt(gomock.Any(), "r0", path).
		Return(nil, errors.New("path absent at revision"))
	revisions.EXPECT().ReadAt(gomock.Any(), "r1", path).Return([]byte("one\n"), nil)

	cutter := NewCutter(source, revisions, []Chunker{lineChunker{}})

	pieces, err := cutter.Pieces(t.Context(), snapshotOf("r0", "r1"))
	if err != nil {
		t.Fatalf("Pieces() = %v", err)
	}

	if born := byName(pieces)["one"].BornAt; born != 1 {
		t.Fatalf("BornAt = %d, want 1 (r0 holds no version to chunk)", born)
	}
}

// The history a caller hands over is not promised to be sorted, and the
// earliest position is the answer. Reading the first match instead of the
// minimum dates a piece at whichever commit happened to come first in the
// slice.
func TestCutterTakesTheEarliestPositionFromAnUnsortedHistory(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	const path = "queries.txt"

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), path).Return([]byte("one\n"), nil)

	revisions := NewMockRevisions(ctrl)
	revisions.EXPECT().ReadAt(gomock.Any(), "late", path).Return([]byte("one\n"), nil).AnyTimes()
	revisions.EXPECT().ReadAt(gomock.Any(), "early", path).Return([]byte("one\n"), nil).AnyTimes()

	snap := entities.Snapshot{
		Files: []string{path},
		History: []entities.SourceTouch{
			{Position: 7, Rev: "late", Touched: []string{path}},
			{Position: 2, Rev: "early", Touched: []string{path}},
		},
	}

	cutter := NewCutter(source, revisions, []Chunker{lineChunker{}})

	pieces, err := cutter.Pieces(t.Context(), snap)
	if err != nil {
		t.Fatalf("Pieces() = %v", err)
	}

	piece := byName(pieces)["one"]
	if piece.BornAt != 2 || piece.BornRev != "early" {
		t.Fatalf("BornAt/BornRev = %d/%q, want 2/\"early\"", piece.BornAt, piece.BornRev)
	}
}

// A file no chunker claims contributes nothing, and must not stop the files
// that follow it from being cut.
func TestCutterSkipsAFileNoChunkerClaims(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), "queries.txt").Return([]byte("one\n"), nil)

	revisions := NewMockRevisions(ctrl)
	revisions.EXPECT().ReadAt(gomock.Any(), "r0", "queries.txt").Return([]byte("one\n"), nil)

	snap := entities.Snapshot{
		Files: []string{"main.go", "queries.txt"},
		History: []entities.SourceTouch{
			{Position: 0, Rev: "r0", Touched: []string{"main.go", "queries.txt"}},
		},
	}

	cutter := NewCutter(source, revisions, []Chunker{lineChunker{}})

	pieces, err := cutter.Pieces(t.Context(), snap)
	if err != nil {
		t.Fatalf("Pieces() = %v", err)
	}

	if len(pieces) != 1 {
		t.Fatalf("got %d pieces, want only the one from queries.txt", len(pieces))
	}
}

func TestCutterFailsWhenAFileCannotBeRead(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	errRead := errors.New("no such object")

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), "queries.txt").Return(nil, errRead)

	cutter := NewCutter(source, NewMockRevisions(ctrl), []Chunker{lineChunker{}})

	_, err := cutter.Pieces(t.Context(), snapshotOf("r0"))
	if !errors.Is(err, errRead) {
		t.Fatalf("Pieces() err = %v, want it to wrap %v", err, errRead)
	}
}
