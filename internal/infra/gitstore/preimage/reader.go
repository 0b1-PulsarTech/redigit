package preimage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

var (
	ErrNoCommit = errors.New("revision names no commit")

	_ anchoring.Revisions = (*Reader)(nil)
)

type Reader struct {
	repo   string
	stream *batch.Stream
}

func NewReader(repo string) *Reader {
	return &Reader{repo: repo, stream: batch.NewStream(repo)}
}

func (reader *Reader) Revision(ctx context.Context, rev string) (anchoring.Revision, error) {
	subject, err := reader.subject(ctx, rev)
	if err != nil {
		return anchoring.Revision{}, err
	}

	// "rev^!" is the commit alone, so the log walks one revision and its diff against the parent.
	for commit, logErr := range batch.Log(ctx, reader.repo, rev+"^!") {
		if logErr != nil {
			return anchoring.Revision{}, fmt.Errorf("read changes of %s: %w", rev, logErr)
		}

		return anchoring.Revision{Subject: subject, Files: changesOf(commit.Entries)}, nil
	}

	return anchoring.Revision{}, fmt.Errorf("%w: %s", ErrNoCommit, rev)
}

func (reader *Reader) Blob(ctx context.Context, oid string) ([]byte, error) {
	out, err := reader.stream.Read(ctx, oid)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", oid, err)
	}

	return out, nil
}

func (reader *Reader) Close(context.Context) error {
	if err := reader.stream.Close(); err != nil {
		return fmt.Errorf("close blob stream: %w", err)
	}

	return nil
}

func (reader *Reader) subject(ctx context.Context, rev string) (string, error) {
	// #nosec G204 -- driving the git CLI is this adapter's purpose.
	cmd := exec.CommandContext(
		ctx, "git", "-C", reader.repo, "log", "-1", "--format=%s", "--end-of-options", rev,
	)

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read subject of %s: %w", rev, err)
	}

	return string(bytes.TrimSpace(out)), nil
}

func changesOf(entries []batch.Entry) []anchoring.FileChange {
	changes := make([]anchoring.FileChange, 0, len(entries))

	for _, entry := range entries {
		changes = append(changes, anchoring.FileChange{
			Path: entry.NewPath, OldPath: entry.OldPath,
			Blob: blobOf(entry.NewBlob), OldBlob: blobOf(entry.OldBlob),
		})
	}

	return changes
}

func blobOf(oid string) string {
	if strings.Trim(oid, "0") == "" {
		return ""
	}

	return oid
}
