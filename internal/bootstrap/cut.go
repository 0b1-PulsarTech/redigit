package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/chunking"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/chunk"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/buildtags"
)

// Cut reads the tree at cfg.Rev and returns every piece of every file a
// chunker claims, dated against that revision's own history. It starts no
// language server: chunking needs no semantic resolution, and a caller who
// wants only the pieces should not pay for gopls to index the repository.
func Cut(ctx context.Context, cfg Config) (pieces []chunking.Piece, err error) {
	reader := gitstore.NewReader(cfg.Repo, cfg.Rev)
	defer func() { err = errors.Join(err, reader.Close()) }()

	snap, snapErr := reader.Snapshot(ctx)
	if snapErr != nil {
		return nil, fmt.Errorf("read snapshot: %w", snapErr)
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

// unseen names the files this run's build tags leave invisible to the
// reference queries. It is computed here, at the composition root, because
// which constraints a tag satisfies is a per-language question and the
// auditing use case knows no language.
func unseen(snap entities.Snapshot, tags []string) ([]auditing.Unseen, error) {
	// Which constraints a tag satisfies is a per-language question the auditing use case cannot
	// answer, so the composition root computes the hidden files.
	constrained, err := buildtags.BuildConstraints(snap)
	if err != nil {
		return nil, fmt.Errorf("read build constraints: %w", err)
	}

	hidden, err := buildtags.Hidden(constrained, tags)
	if err != nil {
		return nil, fmt.Errorf("evaluate build constraints: %w", err)
	}

	out := make([]auditing.Unseen, 0, len(hidden))
	for _, tag := range hidden {
		out = append(out, auditing.Unseen{
			File:       tag.File,
			Constraint: tag.Constraint,
			Tags:       tag.Tags,
		})
	}

	return out, nil
}

// unclaimed names every final-tree path no configured language's extension
// list matches. It is computed here, at the composition root, for the same
// reason unseen is: which extensions a language owns is a per-language
// question, and the auditing use case knows no language.
//
// This is the loud version of the indexer's own read loop
// (internal/domain/usecases/indexing/indexer.go), which skips exactly these
// paths with a bare continue and reports nothing - a plan built from this
// run can never reproduce one of them, and without this, nothing says so
// until the final-tree invariant fails at the very end.
func unclaimed(snap entities.Snapshot, langs []indexing.Language) []auditing.Unclaimed {
	// The indexer's read loop skips these paths silently, so a plan can never reproduce them;
	// naming them here, where extensions are known, is the only warning before the final tree.
	out := make([]auditing.Unclaimed, 0, len(snap.Files))

	for _, file := range snap.Files {
		if !slices.ContainsFunc(
			langs,
			func(lang indexing.Language) bool { return lang.Owns(file) },
		) {
			out = append(out, auditing.Unclaimed{File: file})
		}
	}

	return out
}
