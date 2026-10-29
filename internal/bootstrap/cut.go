package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/chunking"
	"github.com/wrapped-owls/redigit/internal/infra/chunk"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
)

// Cut reads the tree at cfg.Rev and returns every piece of every file a
// chunker claims, dated against that revision's own history. It starts no
// language server: chunking needs no semantic resolution, and a caller who
// wants only the pieces should not pay for gopls to index the repository.
func Cut(ctx context.Context, cfg Config) (pieces []chunking.Piece, err error) {
	reader := gitstore.NewReader(cfg.Repo, cfg.Rev)
	defer func() { err = errors.Join(err, reader.Close()) }()

	snap, err := reader.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	cutter := chunking.NewCutter(reader, reader, chunkers(cfg.ChunkMap))

	pieces, err = cutter.Pieces(ctx, snap)
	if err != nil {
		return nil, fmt.Errorf("cut pieces: %w", err)
	}

	return pieces, nil
}

// chunkers adapts the infra registry to the ports chunking declares. WholeFile
// is left out: it claims every path, which would report one whole-file piece
// per Go file - noise dressed as a chunk. chunkMap, when non-empty, is
// prepended so its entries get first refusal; validateChunkMap stops it
// naming a path another chunker here would already claim.
func chunkers(chunkMap entities.ChunkMap) []chunking.Chunker {
	out := make([]chunking.Chunker, 0, len(chunk.Registry)+1)

	if len(chunkMap) > 0 {
		out = append(out, chunk.MapApplier{Map: chunkMap})
	}

	for _, chunker := range chunk.Registry {
		if _, isWholeFile := chunker.(chunk.WholeFile); isWholeFile {
			continue
		}

		out = append(out, chunker)
	}

	return out
}

// configChunkers is what the config and opaque languages read with, WholeFile
// included: an owned file no chunker claims still has to reach the tree. See
// chunkers for why chunkMap is prepended.
func configChunkers(chunkMap entities.ChunkMap) []config.Chunker {
	out := make([]config.Chunker, 0, len(chunk.Registry)+1)

	if len(chunkMap) > 0 {
		out = append(out, chunk.MapApplier{Map: chunkMap})
	}

	for _, chunker := range chunk.Registry {
		out = append(out, chunker)
	}

	return out
}
