package gitstore

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

func (reader Reader) Changes(ctx context.Context) iter.Seq2[indexing.RevisionChange, error] {
	return func(yield func(indexing.RevisionChange, error) bool) {
		for commit, err := range batch.Log(ctx, reader.RepoPath, reader.Ref) {
			if err != nil {
				yield(indexing.RevisionChange{}, fmt.Errorf("read log: %w", err))

				return
			}

			change := indexing.RevisionChange{
				Rev:   commit.OID,
				Files: make([]indexing.ChangedFile, 0, len(commit.Entries)),
			}

			for _, entry := range commit.Entries {
				change.Files = append(change.Files, indexing.ChangedFile{
					Path: entry.NewPath, Blob: entry.NewBlob, IsDeleted: entry.IsDelete(),
				})
			}

			if !yield(change, nil) {
				return
			}
		}
	}
}

func (reader Reader) ReadBlob(ctx context.Context, blob string) ([]byte, error) {
	out, err := reader.read(ctx, blob)
	isUnreadable := errors.Is(err, batch.ErrMissing) || errors.Is(err, batch.ErrNotBlob) ||
		errors.Is(err, batch.ErrTooLarge)

	if isUnreadable {
		return nil, fmt.Errorf("read blob: %w: %w", indexing.ErrBlobMissing, err)
	}

	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}

	return out, nil
}
