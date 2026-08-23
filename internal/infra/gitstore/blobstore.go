package gitstore

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	Put(ctx context.Context, path string, mode FileMode, content []byte) (oid string, err error)
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
	mode FileMode,
	content []byte,
) (string, error) {
	if !mode.Valid() {
		return "", &UnsupportedModeError{Path: path, Mode: mode}
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

// start launches the hash-object process, deliberately outside any single
// Put call's context: the process outlives whichever call happens to trigger
// it and is torn down only by Close. Tying it to a per-call context instead
// kills it the moment that one call's context ends, even mid-batch.
func (s *BlobStore) start() error {
	if s.cmd != nil {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "redigit-blobstore-")
	if err != nil {
		return fmt.Errorf("create blob store scratch dir: %w", err)
	}

	args := []string{"-C", s.repoPath, "hash-object", "--stdin-paths"}
	if s.write {
		args = append(args, "-w")
	}

	// #nosec G204 -- driving the git CLI is this adapter's purpose; repoPath
	// is caller-supplied infra config, not untrusted input.
	//
	// context.Background(), not a Put call's ctx: this process outlives
	// whichever call happens to start it, and is torn down only by Close.
	cmd := exec.CommandContext(context.Background(), "git", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = os.RemoveAll(tmpDir)

		return fmt.Errorf("open hash-object stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		// cmd.StdinPipe()'s own cleanup only runs via cmd.Start() failing or
		// cmd.Wait() succeeding, neither of which happens here: close it
		// explicitly so this error path does not leak the pipe's fd.
		_ = stdin.Close()
		_ = os.RemoveAll(tmpDir)

		return fmt.Errorf("open hash-object stdout: %w", err)
	}

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	if err = cmd.Start(); err != nil {
		_ = os.RemoveAll(tmpDir)

		return fmt.Errorf("start hash-object: %w", err)
	}

	s.tmpDir = tmpDir
	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewScanner(stdout)
	s.stderr = &stderr

	return nil
}

// hash writes content to a scratch file and asks the running hash-object
// process for its id. A scratch file is unavoidable: --stdin-paths reads
// paths, not content, which is the price paid for batching thousands of
// blobs through one process instead of one process per blob.
func (s *BlobStore) hash(content []byte) (string, error) {
	path, err := s.stage(content)
	if err != nil {
		return "", err
	}

	defer func() { _ = os.Remove(path) }()

	if _, err = fmt.Fprintln(s.stdin, path); err != nil {
		return "", fmt.Errorf("request blob id: %w", err)
	}

	if !s.stdout.Scan() {
		if err = s.stdout.Err(); err != nil {
			return "", fmt.Errorf("read blob id: %w", err)
		}

		return "", fmt.Errorf("read blob id: %w", io.ErrUnexpectedEOF)
	}

	return s.stdout.Text(), nil
}

func (s *BlobStore) stage(content []byte) (string, error) {
	s.next++
	path := filepath.Join(s.tmpDir, fmt.Sprintf("%d", s.next))

	if err := os.WriteFile(path, content, scratchFilePerm); err != nil {
		return "", fmt.Errorf("stage blob content: %w", err)
	}

	return path, nil
}

// PathEscapesDirError is a path that resolves outside a DirSink's base
// directory once joined and cleaned (e.g. via "../" segments). BlobSink is a
// public interface taking a caller-supplied path with no documented trust
// boundary, and a manifest is exactly the kind of artifact that could come
// from another tool or be hand-edited before being fed to a DirSink.
type PathEscapesDirError struct {
	Path string
}

func (e *PathEscapesDirError) Error() string { return "path escapes sink directory" }

func (e *PathEscapesDirError) Unwrap() error { return errPathEscapesDir }

var errPathEscapesDir = errors.New("path escapes directory")

// DirSink writes a plan's blobs onto disk under Dir instead of into a git
// object database, so a caller who does not want the tool touching a
// repository's objects at all still gets the same manifest, minus the oid.
type DirSink struct {
	Dir string
}

var _ BlobSink = (*DirSink)(nil)

func NewDirSink(dir string) DirSink { return DirSink{Dir: dir} }

// Put writes content to Dir/path, creating any parent directories, and
// carries mode into the file's permission bits so an inspected checkout at
// least looks like what the manifest describes. It never returns an object
// id: there is no object database backing a directory.
//
// Whatever already exists at the destination is removed first, so a path
// that changes mode across commits (file to executable, symlink to plain
// file, or the reverse) always lands cleanly instead of silently keeping
// stale permission bits or a stale symlink: os.WriteFile only applies its
// perm argument when creating a file, not when overwriting one.
func (s DirSink) Put(
	_ context.Context,
	path string,
	mode FileMode,
	content []byte,
) (string, error) {
	if !mode.Valid() {
		return "", &UnsupportedModeError{Path: path, Mode: mode}
	}

	base := filepath.Clean(s.Dir)
	full := filepath.Join(base, filepath.FromSlash(path))

	// A path lexically equal to base ("", ".", "sub/..") must be refused too:
	// letting it through turns the RemoveAll below into RemoveAll(base).
	//
	// simplification: lexical only, does not resolve a symlink an earlier
	// Put planted; upgrade path: resolve via filepath.EvalSymlinks first.
	if !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return "", &PathEscapesDirError{Path: path}
	}

	if err := os.MkdirAll(filepath.Dir(full), dirSinkDirPerm); err != nil {
		return "", fmt.Errorf("create blob directory: %w", err)
	}

	if err := os.RemoveAll(full); err != nil {
		return "", fmt.Errorf("remove existing blob: %w", err)
	}

	if mode == ModeSymlink {
		if err := os.Symlink(string(content), full); err != nil {
			return "", fmt.Errorf("symlink blob: %w", err)
		}

		return "", nil
	}

	if err := os.WriteFile(full, content, filePermission(mode)); err != nil {
		return "", fmt.Errorf("write blob: %w", err)
	}

	return "", nil
}

func filePermission(mode FileMode) os.FileMode {
	if mode == ModeExecutable {
		return dirSinkExecFilePerm
	}

	return dirSinkFilePerm
}
