package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/chunking"
)

// pieceDTO is one line of the chunk report: one piece of one file, and where
// the published history first held it. BornAt is -1 when it held it never.
type pieceDTO struct {
	File    string `json:"file"`
	Name    string `json:"name"`
	Chunker string `json:"chunker"`
	Unit    string `json:"unit"`
	Span    [2]int `json:"span"`
	BornAt  int    `json:"born_at"`
	BornRev string `json:"born_rev,omitempty"`
}

// cutter builds the pieces for one run. dispatch wires it to bootstrap.Cut;
// a test wires it to a canned list, the same seam every other command gets
// through starter.
type cutter func(ctx context.Context, cfg bootstrap.Config) ([]chunking.Piece, error)

func runChunks(ctx context.Context, cfg bootstrap.Config, cut cutter, out io.Writer) error {
	pieces, err := cut(ctx, cfg)
	if err != nil {
		return fmt.Errorf("cut files into pieces: %w", err)
	}

	encoder := json.NewEncoder(out)

	for _, piece := range pieces {
		if err = encoder.Encode(toPieceDTO(piece)); err != nil {
			return fmt.Errorf("write chunk line: %w", err)
		}
	}

	return nil
}

func toPieceDTO(piece chunking.Piece) pieceDTO {
	return pieceDTO{
		File:    piece.Decl.Key.File,
		Name:    piece.Decl.Key.Name,
		Chunker: piece.Chunker,
		Unit:    string(piece.Decl.Unit),
		Span:    [2]int{piece.Decl.Span.Start, piece.Decl.Span.End},
		BornAt:  int(piece.BornAt),
		BornRev: piece.BornRev,
	}
}

func toKeys(in []entities.DeclKey) []string {
	out := make([]string, 0, len(in))
	for _, key := range in {
		out = append(out, key.String())
	}

	return out
}
