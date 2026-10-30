package gitstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

// InvariantError is the final-tree invariant failing: the written ref's tree
// does not equal the target tree. Diverging holds the paths git diff-tree
// found different, kept structured rather than folded into Error() so a
// caller can inspect them without parsing free text.
type InvariantError struct {
	Diverging []string
}

func (e *InvariantError) Error() string {
	return fmt.Sprintf("final tree invariant failed: %d paths diverge", len(e.Diverging))
}

func (e *InvariantError) Unwrap() error { return apperr.ErrInvariant }

// Check verifies the final-tree invariant: gotRef's tree must equal wantRef's
// tree. Comparing tree object ids rather than commit shas, because a rewrite
// changes every commit sha even when the tree it produces is exactly right.
func Check(ctx context.Context, repoPath, gotRef, wantRef string) error {
	for _, ref := range []string{gotRef, wantRef} {
		if strings.HasPrefix(ref, "-") {
			return fmt.Errorf("%w: a ref may not start with a dash", ErrBadRev)
		}
	}

	gotTree, err := treeOf(ctx, repoPath, gotRef)
	if err != nil {
		return fmt.Errorf("resolve written tree: %w", err)
	}

	wantTree, err := treeOf(ctx, repoPath, wantRef)
	if err != nil {
		return fmt.Errorf("resolve target tree: %w", err)
	}

	if gotTree == wantTree {
		return nil
	}

	diverging, err := divergingPaths(ctx, repoPath, gotTree, wantTree)
	if err != nil {
		return fmt.Errorf("list diverging paths: %w", err)
	}

	return &InvariantError{Diverging: diverging}
}

func treeOf(ctx context.Context, repoPath, ref string) (string, error) {
	return revParse(ctx, repoPath, ref+"^{tree}")
}

func divergingPaths(ctx context.Context, repoPath, treeA, treeB string) ([]string, error) {
	args := []string{"diff-tree", "-r", "-z", "--name-only", "--end-of-options", treeA, treeB}

	out, err := runGit(ctx, repoPath, args...)
	if err != nil {
		return nil, err
	}

	return splitNUL(out), nil
}

var ErrBadHandle = errors.New("bad blob handle")

type blobFiles struct {
	root *os.Root
	seen map[string][]byte
}

func openBlobFiles(dir string) (*blobFiles, error) {
	if dir == "" {
		return &blobFiles{}, nil
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open blob directory: %w", err)
	}

	return &blobFiles{root: root, seen: map[string][]byte{}}, nil
}

func (files *blobFiles) close() {
	if files.root != nil {
		_ = files.root.Close()
	}
}

func (files *blobFiles) write(blob rewriting.BlobRef) (FileWrite, error) {
	write := FileWrite{Path: blob.Path, Mode: FileMode(blob.Mode)}

	if files.root == nil {
		if !isOid(blob.Handle) {
			return FileWrite{}, fmt.Errorf("%w: not an object id", ErrBadHandle)
		}

		write.Oid = blob.Handle

		return write, nil
	}

	if !filepath.IsLocal(blob.Handle) {
		return FileWrite{}, fmt.Errorf("%w: outside the blob directory", ErrBadHandle)
	}

	content, err := files.read(blob.Handle)
	if err != nil {
		return FileWrite{}, err
	}

	write.Content = content

	return write, nil
}

func (files *blobFiles) read(handle string) ([]byte, error) {
	// Read once and shared: resident memory is the unique blobs, not one copy per row.
	if content, isCached := files.seen[handle]; isCached {
		return content, nil
	}

	// Non-blocking so a FIFO cannot hang the open; the kind is checked on the open
	// descriptor, so the file cannot be swapped between the check and the read.
	file, err := files.root.OpenFile(handle, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, readFailure(err)
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, readFailure(err)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file", ErrBadHandle)
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, readFailure(err)
	}

	files.seen[handle] = content

	return content, nil
}

func readFailure(err error) error {
	// os exports no sentinel for an os.Root escape, only this text on the PathError.
	pathErr, isPath := errors.AsType[*fs.PathError](err)
	if isPath && pathErr.Err.Error() == "path escapes from parent" {
		return fmt.Errorf("%w: outside the blob directory", ErrBadHandle)
	}

	return fmt.Errorf("read blob file: %w", err)
}
