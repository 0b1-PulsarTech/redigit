package bootstrap

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/chunking"
	"github.com/wrapped-owls/redigit/internal/infra/chunk"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
)

// Cut reads the tree at cfg.Rev and returns every piece of every file a
// chunker claims, dated against that revision's own history. It starts no
// language server: chunking needs no semantic resolution, and a caller who
// wants only the pieces should not pay for gopls to index the repository.
func Cut(ctx context.Context, cfg Config) ([]chunking.Piece, error) {
	reader := gitstore.NewReader(cfg.Repo, cfg.Rev)

	snap, err := reader.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	cutter := chunking.NewCutter(reader, reader, chunkers())

	pieces, err := cutter.Pieces(ctx, snap)
	if err != nil {
		return nil, fmt.Errorf("cut pieces: %w", err)
	}

	return pieces, nil
}

// chunkers adapts the infra registry to the ports chunking declares. WholeFile
// is left out on purpose: it claims every path, so including it would report
// one whole-file piece for every Go file in the tree, which is noise dressed
// as a chunk.
func chunkers() []chunking.Chunker {
	out := make([]chunking.Chunker, 0, len(chunk.Registry))

	for _, chunker := range chunk.Registry {
		if _, isWholeFile := chunker.(chunk.WholeFile); isWholeFile {
			continue
		}

		out = append(out, chunker)
	}

	return out
}

// configChunkers is what the config language reads with, WholeFile included:
// an owned file no chunker claims still has to reach the tree.
func configChunkers() []config.Chunker {
	out := make([]config.Chunker, 0, len(chunk.Registry))
	for _, chunker := range chunk.Registry {
		out = append(out, chunker)
	}

	return out
}
