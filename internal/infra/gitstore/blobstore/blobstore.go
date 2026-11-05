package blobstore

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

// BlobSink stores one file's content for a manifest to reference: either into
// a repository's object database (content-addressed, deduplicated by git) or
// onto disk under a plan directory (path-addressed, no object database).
//
// Put returns the object id a content-addressed sink assigned, or "" for a
// sink that has none. path and mode never change a blob's identity, but a
// disk-backed sink needs them to pick a destination and a permission bit, so
// every implementation takes the same three arguments.
type BlobSink interface {
	Put(
		ctx context.Context,
		path string,
		mode gitstore.FileMode,
		content []byte,
	) (oid string, err error)
}

// hashObjectError is `git hash-object` exiting non-zero. Its stderr is kept
// as a field rather than folded into Error(), the same reason commandError
// and fastImportError keep it separate: a low-cardinality message a caller
// can log without carrying whatever git happened to print.
type hashObjectError struct {
	Stderr string
	cause  error
}

func (e *hashObjectError) Error() string {
	return fmt.Sprintf("run hash-object: %s", e.cause.Error())
}

func (e *hashObjectError) Unwrap() error { return e.cause }

// Scratch and destination file permissions. Owner-only: a blob store's
// scratch directory and a DirSink's output are working files for this tool
// and whatever git process it drives, not shared artifacts.
const (
	scratchFilePerm     = 0o600
	dirSinkDirPerm      = 0o750
	dirSinkExecFilePerm = 0o700
	dirSinkFilePerm     = 0o600
)

// BlobStore writes file content into a repository's object database through
// one long-lived `git hash-object --stdin-paths` process, the same
// long-lived-process shape Writer already uses for fast-import: a per-blob
// subprocess loop measured 77x slower than one process fed thousands of
// requests over its lifetime. --stdin-paths was chosen over --stdin because
// --stdin hashes exactly one blob per invocation, which is that same
// per-blob-process shape in disguise; --stdin-paths reads one path per line
// and prints one id per line, so a single process serves the whole run.
//
// Writing a loose object is additive: it touches no ref, no index, and no
// working tree, which is why it is safe to do against a repository this
// tool does not own.
type BlobStore struct {
	repoPath string
	write    bool // false previews ids without ever invoking -w

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	stderr *bytes.Buffer
	tmpDir string
	next   int
	seen   map[string]string // raw content -> already-computed oid
}

var _ BlobSink = (*BlobStore)(nil)

// NewBlobStore writes blobs into repoPath's object database as they are Put.
func NewBlobStore(repoPath string) *BlobStore {
	return &BlobStore{repoPath: repoPath, write: true, seen: make(map[string]string)}
}

// NewDryRunBlobStore computes the object id Put would assign without writing
// any object, so a caller can preview a manifest before committing to it.
func NewDryRunBlobStore(repoPath string) *BlobStore {
	return &BlobStore{repoPath: repoPath, write: false, seen: make(map[string]string)}
}

// Put returns content's git blob object id, writing the object unless the
// store was built with NewDryRunBlobStore. Identical content returns the same
// id without a second round trip through the process: the first call to see a
// given byte sequence pays for hashing it, every later one is a map lookup.
func (s *BlobStore) Put(
	ctx context.Context,
	path string,
	mode gitstore.FileMode,
	content []byte,
) (string, error) {
	if !mode.Valid() {
		return "", &gitstore.UnsupportedModeError{Path: path, Mode: mode}
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("put blob: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(content)
	if oid, ok := s.seen[key]; ok {
		return oid, nil
	}

	if err := s.start(); err != nil {
		return "", err
	}

	oid, err := s.hash(content)
	if err != nil {
		return "", err
	}

	s.seen[key] = oid

	return oid, nil
}

// Close shuts down the hash-object process and removes the scratch directory
// Put staged content in. It is a no-op when Put was never called.
func (s *BlobStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd == nil {
		return nil
	}

	// Check waitErr first: a process that crashes on its own usually closes
	// its end of the pipe before exiting, which turns stdin.Close() into a
	// generic broken-pipe error and would otherwise bury the real diagnostic
	// (held in s.stderr) inside waitErr instead of surfacing it.
	closeErr := s.stdin.Close()
	waitErr := s.cmd.Wait()
	removeErr := os.RemoveAll(s.tmpDir)

	// Priority waitErr > closeErr > removeErr: git's own failure (with its
	// real stderr) and the stdin close both diagnose the hash-object process
	// itself, and either one matters more to a caller than a scratch
	// directory this tool created and no longer needs.
	if waitErr != nil {
		return &hashObjectError{Stderr: s.stderr.String(), cause: waitErr}
	}

	if closeErr != nil {
		return fmt.Errorf("close hash-object stdin: %w", closeErr)
	}

	if removeErr != nil {
		return fmt.Errorf("remove blob store scratch dir: %w", removeErr)
	}

	return nil
}
