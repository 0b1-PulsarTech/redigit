package batch

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
)

var _ lineage.RevisionLog = (*Revisions)(nil)

type Revisions struct {
	repo   string
	ref    string
	stream *Stream
}

func NewRevisions(repo, ref string) *Revisions {
	return &Revisions{repo: repo, ref: ref, stream: NewStream(repo)}
}

func (revisions *Revisions) Changes(ctx context.Context) iter.Seq2[lineage.Revision, error] {
	return func(yield func(lineage.Revision, error) bool) {
		for commit, err := range Log(ctx, revisions.repo, revisions.ref) {
			if err != nil {
				yield(lineage.Revision{}, fmt.Errorf("read log: %w", err))

				return
			}

			if !yield(revisionOf(commit), nil) {
				return
			}
		}
	}
}

func (revisions *Revisions) ReadBlob(ctx context.Context, oid string) ([]byte, error) {
	out, err := revisions.stream.Read(ctx, oid)
	isUnreadable := errors.Is(err, ErrMissing) || errors.Is(err, ErrNotBlob) ||
		errors.Is(err, ErrTooLarge)

	if isUnreadable {
		return nil, fmt.Errorf("read blob: %w: %w", lineage.ErrBlobMissing, err)
	}

	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}

	return out, nil
}

func (revisions *Revisions) Close() error {
	if err := revisions.stream.Close(); err != nil {
		return fmt.Errorf("close batch stream: %w", err)
	}

	return nil
}

func revisionOf(commit Commit) lineage.Revision {
	changes := make([]lineage.Change, 0, len(commit.Entries))

	for _, entry := range commit.Entries {
		changes = append(changes, lineage.Change{
			Path: entry.NewPath, OldPath: entry.OldPath,
			Blob: entry.NewBlob, OldBlob: entry.OldBlob,
			Status: entry.Status, Similarity: entry.Similarity,
		})
	}

	return lineage.Revision{Rev: commit.OID, Changes: changes}
}
