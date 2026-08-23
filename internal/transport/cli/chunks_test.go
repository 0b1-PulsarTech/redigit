package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/chunking"
)

func pieceOf(file, name string, bornAt entities.Position, bornRev string) chunking.Piece {
	return chunking.Piece{
		Decl: entities.Decl{
			Key:  entities.DeclKey{File: file, Kind: entities.KindOther, Name: name},
			Unit: entities.UnitID("database/queries"),
			Span: entities.Span{Start: 10, End: 40},
		},
		Chunker: "sql-queries",
		BornAt:  bornAt,
		BornRev: bornRev,
	}
}

func TestRunChunksWritesOneLinePerPiece(t *testing.T) {
	t.Parallel()

	pieces := []chunking.Piece{
		pieceOf("database/queries/lead.sql", "GetLead", 12, "abc123"),
		pieceOf("database/queries/lead.sql", "ListLeads", 40, "def456"),
	}

	var out bytes.Buffer

	err := runChunks(
		t.Context(),
		bootstrap.Config{},
		func(context.Context, bootstrap.Config) ([]chunking.Piece, error) { return pieces, nil },
		&out,
	)
	if err != nil {
		t.Fatalf("runChunks() = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != len(pieces) {
		t.Fatalf("got %d lines, want %d", len(lines), len(pieces))
	}

	var first pieceDTO
	if err = json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 1 is not JSON: %v", err)
	}

	testCases := []struct {
		name string
		got  any
		want any
	}{
		{name: "file", got: first.File, want: "database/queries/lead.sql"},
		{name: "name", got: first.Name, want: "GetLead"},
		{name: "chunker", got: first.Chunker, want: "sql-queries"},
		{name: "unit", got: first.Unit, want: "database/queries"},
		{name: "born_at", got: first.BornAt, want: 12},
		{name: "born_rev", got: first.BornRev, want: "abc123"},
		{name: "span start", got: first.Span[0], want: 10},
		{name: "span end", got: first.Span[1], want: 40},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.got != testCase.want {
				t.Fatalf("%s = %v, want %v", testCase.name, testCase.got, testCase.want)
			}
		})
	}
}

// A piece the published history never held carries NoPosition, and that must
// survive the JSON round trip as -1. Emitting it as 0 would tell the reading
// agent to commit the piece first, which is the earliest position there is.
func TestRunChunksKeepsNoPositionDistinctFromZero(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	err := runChunks(
		t.Context(),
		bootstrap.Config{},
		func(context.Context, bootstrap.Config) ([]chunking.Piece, error) {
			return []chunking.Piece{pieceOf("go.mod", "require github.com/x", entities.NoPosition, "")}, nil
		},
		&out,
	)
	if err != nil {
		t.Fatalf("runChunks() = %v", err)
	}

	var got pieceDTO
	if err = json.Unmarshal([]byte(strings.TrimSpace(out.String())), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}

	if got.BornAt != int(entities.NoPosition) {
		t.Fatalf("born_at = %d, want %d", got.BornAt, entities.NoPosition)
	}

	if strings.Contains(out.String(), `"born_rev"`) {
		t.Fatalf("born_rev is present for a piece with no birth: %s", out.String())
	}
}

func TestRunChunksFailsWhenCuttingFails(t *testing.T) {
	t.Parallel()

	errCut := errors.New("no such revision")

	var out bytes.Buffer

	err := runChunks(
		t.Context(),
		bootstrap.Config{},
		func(context.Context, bootstrap.Config) ([]chunking.Piece, error) { return nil, errCut },
		&out,
	)

	if !errors.Is(err, errCut) {
		t.Fatalf("err = %v, want it to wrap %v", err, errCut)
	}
}
