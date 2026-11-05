package gitstore

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

var (
	_ indexing.Tree        = Reader{}
	_ indexing.RevisionLog = Reader{}
)

// ErrAbsentAtRevision reports a path a revision does not hold.
var ErrAbsentAtRevision = errors.New("path absent at revision")

// Reader reads a Snapshot out of a real git repository at Ref: the final
// tree's file list, the source history's per-position touch list, and a
// path's final content.
type Reader struct {
	RepoPath string
	Ref      string
	stream   *batch.Stream
}

func NewReader(repoPath, ref string) Reader {
	return Reader{RepoPath: repoPath, Ref: ref, stream: batch.NewStream(repoPath)}
}

func (reader Reader) Close() error {
	if reader.stream == nil {
		return nil
	}

	if err := reader.stream.Close(); err != nil {
		return fmt.Errorf("close batch stream: %w", err)
	}

	return nil
}

// Snapshot reads the final tree's files and the source history at Ref.
func (reader Reader) Snapshot(ctx context.Context) (entities.Snapshot, error) {
	files, err := reader.finalFiles(ctx)
	if err != nil {
		return entities.Snapshot{}, fmt.Errorf("list final tree: %w", err)
	}

	history, err := reader.history(ctx)
	if err != nil {
		return entities.Snapshot{}, fmt.Errorf("read source history: %w", err)
	}

	return entities.Snapshot{Root: reader.RepoPath, Files: files, History: history}, nil
}

// Read returns path's content in the final tree at Ref, which is what every
// intermediate render is cut from rather than an intermediate blob.
func (reader Reader) Read(ctx context.Context, path string) ([]byte, error) {
	out, err := reader.read(ctx, reader.Ref+":"+path)
	if err != nil {
		return nil, fmt.Errorf("read path content: %w", err)
	}

	return out, nil
}

// Modes reports the blob mode of every path in the final tree. A tree hashes
// the mode with the content, so rebuilding an executable script as a plain file
// yields a different tree and the final-tree invariant fails at the end without
// naming why.
func (reader Reader) Modes(ctx context.Context) (map[string]FileMode, error) {
	entries, err := reader.entries(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[string]FileMode, len(entries))
	for _, entry := range entries {
		out[entry.path] = entry.mode
	}

	return out, nil
}

// ReadAt returns a path's content at one revision. A path the revision does
// not hold reports ErrAbsentAtRevision rather than an empty file, so a caller
// scanning history can tell "deleted here" from "empty here".
func (reader Reader) ReadAt(ctx context.Context, rev, path string) ([]byte, error) {
	out, err := reader.read(ctx, rev+":"+path)
	if errors.Is(err, batch.ErrMissing) {
		return nil, fmt.Errorf("%s at %s: %w", path, rev, ErrAbsentAtRevision)
	}

	if err != nil {
		return nil, fmt.Errorf("read path at revision: %w", err)
	}

	return out, nil
}

func (reader Reader) finalFiles(ctx context.Context) ([]string, error) {
	entries, err := reader.entries(ctx)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry.path)
	}

	slices.Sort(files)

	return files, nil
}
