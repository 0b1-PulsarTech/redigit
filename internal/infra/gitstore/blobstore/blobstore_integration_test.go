//go:build integration

package blobstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

// emptyBlobOID is git's well-known object id for a zero-byte blob: the SHA-1
// of "blob 0\0". Any correctly-computed store must reproduce it exactly.
const emptyBlobOID = "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"

func catFile(t *testing.T, dir, oid string) []byte {
	t.Helper()

	out, err := blobGit(t.Context(), dir, "cat-file", "-p", oid)
	if err != nil {
		t.Fatalf("cat-file -p %s: %v", oid, err)
	}

	return out
}

func objectExists(t *testing.T, dir, oid string) bool {
	t.Helper()

	_, err := blobGit(t.Context(), dir, "cat-file", "-e", oid)

	return err == nil
}

func blobGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return nil, fmt.Errorf("git %v: %w", args, err)
	}

	return out, nil
}

func blobHead(ctx context.Context, dir string) (string, error) {
	out, err := blobGit(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return string(bytes.TrimSpace(out)), nil
}

func blobTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Tester"},
	} {
		if _, err := blobGit(t.Context(), dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	return dir
}

func blobCommitFiles(t *testing.T, dir, message string, files map[string][]byte) {
	t.Helper()

	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))

		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}

		if err := os.WriteFile(full, content, 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", message}} {
		if _, err := blobGit(t.Context(), dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
}

func TestBlobStorePutSameContentWritesOnce(t *testing.T) {
	t.Parallel()

	dir := blobTestRepo(t)
	s := NewBlobStore(dir)

	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("Close() err = %v", err)
		}
	})

	content := []byte("package a\n")

	oid1, err := s.Put(t.Context(), "a.go", gitstore.ModeFile, content)
	if err != nil {
		t.Fatalf("Put() err = %v", err)
	}

	oid2, err := s.Put(t.Context(), "b.go", gitstore.ModeFile, content)
	if err != nil {
		t.Fatalf("Put() err = %v", err)
	}

	if oid1 != oid2 {
		t.Fatalf("Put() oids = %s, %s, want the same id for identical content", oid1, oid2)
	}

	if s.next != 1 {
		t.Fatalf(
			"staged %d scratch files, want 1: identical content must not round-trip twice",
			s.next,
		)
	}

	if string(catFile(t, dir, oid1)) != string(content) {
		t.Fatalf("cat-file content mismatch")
	}
}

func TestBlobStorePutEmptyContentIsTheWellKnownID(t *testing.T) {
	t.Parallel()

	dir := blobTestRepo(t)
	s := NewBlobStore(dir)

	t.Cleanup(func() { _ = s.Close() })

	oid, err := s.Put(t.Context(), "empty.go", gitstore.ModeFile, nil)
	if err != nil {
		t.Fatalf("Put() err = %v", err)
	}

	if oid != emptyBlobOID {
		t.Fatalf("Put() oid = %s, want the well-known empty blob id %s", oid, emptyBlobOID)
	}
}

func TestBlobStorePutSurvivesNULAndNonUTF8Content(t *testing.T) {
	t.Parallel()

	dir := blobTestRepo(t)
	s := NewBlobStore(dir)

	t.Cleanup(func() { _ = s.Close() })

	testCases := []struct {
		name    string
		content []byte
	}{
		{"a NUL byte", []byte("before\x00after")},
		{"invalid UTF-8", []byte{0xff, 0xfe, 0x00, 'a', 0xc0, 0xaf}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			oid, err := s.Put(t.Context(), "bin", gitstore.ModeFile, testCase.content)
			if err != nil {
				t.Fatalf("Put() err = %v", err)
			}

			got := catFile(t, dir, oid)
			if string(got) != string(testCase.content) {
				t.Fatalf("cat-file content = %q, want %q", got, testCase.content)
			}
		})
	}
}

func TestBlobStorePutAcceptsPathsWithSpaceAndQuote(t *testing.T) {
	t.Parallel()

	dir := blobTestRepo(t)
	s := NewBlobStore(dir)

	t.Cleanup(func() { _ = s.Close() })

	testCases := []struct {
		name string
		path string
	}{
		{"a path with a space", "sub dir/b.go"},
		{"a path with a quote", `weird"name.go`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// path never enters the object's content or identity: a blob is
			// content-addressed. This proves the store does not choke on it.
			oid, err := s.Put(t.Context(), testCase.path, gitstore.ModeFile, []byte(testCase.path))
			if err != nil {
				t.Fatalf("Put() err = %v", err)
			}

			if string(catFile(t, dir, oid)) != testCase.path {
				t.Fatalf("cat-file content mismatch for path %q", testCase.path)
			}
		})
	}
}

func TestBlobStoreDryRunMatchesWriteAndWritesNothing(t *testing.T) {
	t.Parallel()

	dir := blobTestRepo(t)
	content := []byte("package a\n")

	dry := NewDryRunBlobStore(dir)

	t.Cleanup(func() { _ = dry.Close() })

	dryOID, err := dry.Put(t.Context(), "a.go", gitstore.ModeFile, content)
	if err != nil {
		t.Fatalf("dry run Put() err = %v", err)
	}

	if objectExists(t, dir, dryOID) {
		t.Fatalf("dry run wrote an object, want none written")
	}

	write := NewBlobStore(dir)

	t.Cleanup(func() { _ = write.Close() })

	writeOID, err := write.Put(t.Context(), "a.go", gitstore.ModeFile, content)
	if err != nil {
		t.Fatalf("Put() err = %v", err)
	}

	if dryOID != writeOID {
		t.Fatalf("dry run id = %s, write id = %s, want the same id", dryOID, writeOID)
	}

	if !objectExists(t, dir, writeOID) {
		t.Fatalf("Put() reported an id but did not write the object")
	}
}

func TestBlobStoreNeverTouchesRefIndexOrWorkingTree(t *testing.T) {
	t.Parallel()

	dir := blobTestRepo(t)
	blobCommitFiles(t, dir, "seed", map[string][]byte{"a.go": []byte("package a\n")})

	headBefore, err := blobHead(t.Context(), dir)
	if err != nil {
		t.Fatalf("resolve HEAD before: %v", err)
	}

	s := NewBlobStore(dir)

	for i := range 50 {
		content := fmt.Appendf(nil, "content %d\n", i)
		if _, err = s.Put(t.Context(), fmt.Sprintf("f%d.go", i), gitstore.ModeFile, content); err != nil {
			t.Fatalf("Put() err = %v", err)
		}
	}

	if err = s.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	headAfter, err := blobHead(t.Context(), dir)
	if err != nil {
		t.Fatalf("resolve HEAD after: %v", err)
	}

	if headBefore != headAfter {
		t.Fatalf(
			"HEAD moved from %s to %s: a blob write must never touch a ref",
			headBefore,
			headAfter,
		)
	}

	status, err := blobGit(t.Context(), dir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("git status: %v", err)
	}

	if len(status) != 0 {
		t.Fatalf(
			"git status --porcelain = %q, want empty: a blob write must touch neither index nor working tree",
			status,
		)
	}
}

func TestBlobStoreOnUnbornBranchTouchesNoRef(t *testing.T) {
	t.Parallel()

	dir := blobTestRepo(t)
	s := NewBlobStore(dir)

	if _, err := s.Put(t.Context(), "a.go", gitstore.ModeFile, []byte("package a\n")); err != nil {
		t.Fatalf("Put() err = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	if _, err := blobHead(t.Context(), dir); err == nil {
		t.Fatalf("HEAD resolved on a repo with no commits: a blob write must never create one")
	}
}

// TestBlobStoreThroughputAgainstRealData writes several hundred blobs of
// varying, sometimes non-UTF-8, content through one BlobStore and verifies
// every one of them, byte for byte, via git cat-file -p. It also logs the
// achieved throughput for a human reading -v output to sanity-check, but it
// asserts nothing about speed: a regression back to the 77x-slower
// per-blob-process shape this batching replaced would still pass this test,
// just quietly slower, unless someone reads the log.
func TestBlobStoreThroughputAgainstRealData(t *testing.T) {
	t.Parallel()

	dir := blobTestRepo(t)
	s := NewBlobStore(dir)

	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("Close() err = %v", err)
		}
	})

	const blobCount = 500

	contents := make([][]byte, blobCount)

	for i := range blobCount {
		switch {
		case i == 0:
			contents[i] = nil // the empty blob, mixed into the batch
		case i%37 == 0:
			// non-UTF-8 content, scattered through the batch rather than
			// isolated, so the real run proves it survives alongside plain text.
			contents[i] = []byte{0xff, 0xfe, byte(i), 0x00, 0xc0, 0xaf}
		default:
			contents[i] = fmt.Appendf(nil, "package p%d\n\nfunc F%d() int { return %d }\n", i, i, i)
		}
	}

	start := time.Now()
	oids := make([]string, blobCount)

	for i, content := range contents {
		oid, err := s.Put(t.Context(), fmt.Sprintf("pkg/p%d.go", i), gitstore.ModeFile, content)
		if err != nil {
			t.Fatalf("Put(%d) err = %v", i, err)
		}

		oids[i] = oid
	}

	elapsed := time.Since(start)
	perSecond := float64(blobCount) / elapsed.Seconds()

	t.Logf(
		"wrote %d blobs via a single long-lived `git hash-object -w --stdin-paths` process in %s (%.0f blobs/sec)",
		blobCount,
		elapsed,
		perSecond,
	)

	for i, oid := range oids {
		got := catFile(t, dir, oid)
		if string(got) != string(contents[i]) {
			t.Fatalf("blob %d: cat-file content mismatch", i)
		}
	}

	status, err := blobGit(t.Context(), dir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("git status: %v", err)
	}

	if len(status) != 0 {
		t.Fatalf("git status --porcelain = %q, want empty after %d blob writes", status, blobCount)
	}

	if _, err = blobHead(t.Context(), dir); err == nil {
		t.Fatalf("HEAD resolved after %d blob writes on a repo with no commits", blobCount)
	}
}

func TestBlobStoreCloseReportsHashObjectFailure(t *testing.T) {
	t.Parallel()

	// dir is a real filesystem directory but not a git repository (no
	// newTestRepo/git init here): `git -C dir hash-object -w --stdin-paths`
	// fails immediately with a real "fatal: not a git repository..." on
	// stderr and exit code 128, before it ever reads stdin. Git validates
	// the repository before touching stdin, so this is deterministic - no
	// timing race against a corrupted pipe to worry about.
	dir := t.TempDir()
	s := NewBlobStore(dir)

	// Put's own return doesn't matter here: either the write to stdin fails
	// because the process already exited, or the read of stdout hits EOF.
	_, _ = s.Put(t.Context(), "a.go", gitstore.ModeFile, []byte("x"))

	err := s.Close()
	if err == nil {
		t.Fatalf("Close() err = nil, want an error from a non-repository directory")
	}

	var hashErr *hashObjectError
	if !errors.As(err, &hashErr) {
		t.Fatalf("Close() err = %v, want *hashObjectError", err)
	}

	if !strings.Contains(hashErr.Stderr, "not a git repository") {
		t.Fatalf(
			"hashObjectError.Stderr = %q, want it to contain %q",
			hashErr.Stderr,
			"not a git repository",
		)
	}
}
