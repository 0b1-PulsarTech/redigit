//go:build integration

package gitstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// twoCommitFixture is a small synthetic history: a first commit that writes
// two paths, and a second that adds one path and deletes another.
func twoCommitFixture() []WriteCommit {
	return []WriteCommit{
		{
			Author:    identityAt(1700000000),
			Committer: identityAt(1700000001),
			Message:   "first",
			Writes: []FileWrite{
				{Path: "a.go", Content: []byte("package a\n")},
				{Path: "sub dir/b.go", Content: []byte("package b\n")},
			},
		},
		{
			Author:    identityAt(1700000010),
			Committer: identityAt(1700000011),
			Message:   "second",
			Writes: []FileWrite{
				{Path: "c.go", Content: []byte("package c\n")},
			},
			Deletes: []string{"sub dir/b.go"},
		},
	}
}

func TestWriterWriteMatchesIndependentTarget(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)

	if _, err := NewWriter(
		dir,
	).Write(t.Context(), "refs/heads/out", twoCommitFixture()); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	// Build the target tree independently of the writer: a plain commit
	// holding exactly the final state (a.go and c.go; sub dir/b.go absent).
	commitTestFiles(t, dir, "target", map[string][]byte{
		"a.go": []byte("package a\n"),
		"c.go": []byte("package c\n"),
	})
	runTestGit(t, dir, "update-ref", "refs/heads/target", "main")

	if err := Check(t.Context(), dir, "refs/heads/out", "refs/heads/target"); err != nil {
		t.Fatalf("Check() err = %v, want the written tree to match the target", err)
	}
}

func TestWriterWriteIsDeterministic(t *testing.T) {
	t.Parallel()

	shaOf := func(dir string) string {
		sha, err := NewWriter(dir).Write(t.Context(), "refs/heads/out", twoCommitFixture())
		if err != nil {
			t.Fatalf("Write() err = %v", err)
		}

		return sha
	}

	sha1 := shaOf(newTestRepo(t))
	sha2 := shaOf(newTestRepo(t))

	if sha1 != sha2 {
		t.Fatalf("Write() is not deterministic: %s != %s", sha1, sha2)
	}
}

func TestWriterWriteDeleteIsGenuinelyAbsent(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)

	if _, err := NewWriter(
		dir,
	).Write(t.Context(), "refs/heads/out", twoCommitFixture()); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	r := NewReader(dir, "refs/heads/out")

	files, err := r.finalFiles(t.Context())
	if err != nil {
		t.Fatalf("finalFiles() err = %v", err)
	}

	for _, path := range files {
		if path == "sub dir/b.go" {
			t.Fatalf("finalFiles() still lists the deleted path, want it absent")
		}
	}

	if _, err := r.Read(t.Context(), "sub dir/b.go"); err == nil {
		t.Fatalf("Read() of the deleted path succeeded, want an error")
	}
}

func TestWriterWriteTwiceStartsFromARootCommit(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	fixture := twoCommitFixture()

	if _, err := NewWriter(dir).Write(t.Context(), "refs/heads/out", fixture); err != nil {
		t.Fatalf("first Write() err = %v", err)
	}

	firstTip, err := revParse(t.Context(), dir, "refs/heads/out")
	if err != nil {
		t.Fatalf("revParse() err = %v", err)
	}

	if _, err = NewWriter(dir).Write(t.Context(), "refs/heads/out", fixture[:1]); err != nil {
		t.Fatalf("second Write() err = %v", err)
	}

	out, err := runGit(t.Context(), dir, "rev-list", "--parents", "-n", "1", "refs/heads/out")
	if err != nil {
		t.Fatalf("rev-list err = %v", err)
	}

	if fields := strings.Fields(string(out)); len(fields) != 1 {
		t.Fatalf(
			"second write's first commit has parents %v (old tip %s), want none",
			fields[1:],
			firstTip,
		)
	}
}

func TestWriterWriteDataref(t *testing.T) {
	t.Parallel()

	const missingOid = "0123456789abcdef0123456789abcdef01234567"

	testCases := []struct {
		name           string
		isHashed       bool
		isStderrWanted bool
	}{
		{"pre-hashed blob lands in the tree", true, false},
		{"missing oid surfaces fast-import stderr", false, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := newTestRepo(t)
			oid := missingOid

			if testCase.isHashed {
				source := filepath.Join(t.TempDir(), "blob")
				if err := os.WriteFile(source, []byte("package a\n"), 0o600); err != nil {
					t.Fatalf("write blob source: %v", err)
				}

				out, err := runGit(t.Context(), dir, "hash-object", "-w", source)
				if err != nil {
					t.Fatalf("hash-object err = %v", err)
				}

				oid = strings.TrimSpace(string(out))
			}

			commits := []WriteCommit{{
				Author: identityAt(1700000000), Committer: identityAt(1700000000), Message: "m",
				Writes: []FileWrite{{Path: "a.go", Oid: oid}},
			}}

			_, err := NewWriter(dir).Write(t.Context(), "refs/heads/out", commits)

			if testCase.isStderrWanted {
				var failure *fastImportError
				if !errors.As(err, &failure) || failure.Stderr == "" {
					t.Fatalf("Write() err = %v, want a fast-import error carrying stderr", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Write() err = %v", err)
			}

			got, err := NewReader(dir, "refs/heads/out").Read(t.Context(), "a.go")
			if err != nil || string(got) != "package a\n" {
				t.Fatalf("Read() = %q, %v, want the hashed content", got, err)
			}
		})
	}
}

func TestWriterWriteLargeStreamKeepsFastImportStderr(t *testing.T) {
	t.Parallel()

	const missingOid = "0123456789abcdef0123456789abcdef01234567"

	testCases := []struct {
		name        string
		paddingSize int
	}{
		{"stream far beyond the pipe buffer", 200 << 10},
		{"stream within the pipe buffer", 1 << 10},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			const paddings = 8

			commits := make([]WriteCommit, 0, paddings+1)
			commits = append(commits, WriteCommit{
				Author: identityAt(1700000000), Committer: identityAt(1700000000), Message: "bad",
				Writes: []FileWrite{{Path: "a.go", Oid: missingOid}},
			})

			for index := range paddings {
				commits = append(commits, WriteCommit{
					Author: identityAt(1700000001), Committer: identityAt(1700000001),
					Message: "padding",
					Writes: []FileWrite{{
						Path:    fmt.Sprintf("pad%d", index),
						Content: bytes.Repeat([]byte{'x'}, testCase.paddingSize/paddings),
					}},
				})
			}

			_, err := NewWriter(newTestRepo(t)).Write(t.Context(), "refs/heads/out", commits)

			var failure *fastImportError
			if !errors.As(err, &failure) || failure.Stderr == "" {
				t.Fatalf("Write() err = %v, want a fast-import error carrying stderr", err)
			}
		})
	}
}

func countPackFiles(t *testing.T, dir string) int {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(dir, ".git", "objects", "pack"))
	if err != nil {
		t.Fatalf("read pack dir: %v", err)
	}

	return len(entries)
}

func TestWriterWriteRefusesInvalidCommitWithoutMovingRef(t *testing.T) {
	t.Parallel()

	badMode := FileWrite{Path: "a.go", Mode: "100664", Content: []byte("a")}
	testCases := []struct {
		name        string
		isPreExists bool
		bad         FileWrite
		wantErr     error
	}{
		{"the ref is never created", false, badMode, errUnsupportedMode},
		{"an existing ref keeps its old value", true, badMode, errUnsupportedMode},
		{"dot-dot path, ref never created", false, FileWrite{Path: "a/../b"}, errBadPath},
		{"git dir path, ref kept", true, FileWrite{Path: ".GIT/config"}, errBadPath},
		{"empty path, ref never created", false, FileWrite{}, errBadPath},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := newTestRepo(t)

			oldTip := ""

			if testCase.isPreExists {
				var err error
				if oldTip, err = NewWriter(dir).Write(
					t.Context(), "refs/heads/out", twoCommitFixture()[:1],
				); err != nil {
					t.Fatalf("seed Write() err = %v", err)
				}
			}

			packsBefore := countPackFiles(t, dir)
			testCase.bad.Content = []byte("a")
			commits := []WriteCommit{
				{
					Author: identityAt(1700000000), Committer: identityAt(1700000000),
					Message: "big",
					Writes: []FileWrite{{
						Path: "big", Content: bytes.Repeat([]byte{'x'}, 16<<10),
					}},
				},
				{
					Author: identityAt(1700000001), Committer: identityAt(1700000001),
					Message: "bad", Writes: []FileWrite{testCase.bad},
				},
			}

			_, err := NewWriter(dir).Write(t.Context(), "refs/heads/out", commits)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Write() err = %v, want %v", err, testCase.wantErr)
			}

			if _, started := errors.AsType[*fastImportError](err); started {
				t.Fatalf("Write() err = %v, want fast-import never started", err)
			}

			if packsAfter := countPackFiles(t, dir); packsAfter != packsBefore {
				t.Fatalf("pack files %d -> %d, want none written", packsBefore, packsAfter)
			}

			got, revErr := revParse(t.Context(), dir, "refs/heads/out")

			if !testCase.isPreExists {
				if revErr == nil {
					t.Fatalf("ref exists at %s, want it absent", got)
				}

				return
			}

			if revErr != nil || got != oldTip {
				t.Fatalf("ref = %q, %v, want the old tip %s", got, revErr, oldTip)
			}
		})
	}
}

func midStreamCommits(t *testing.T, size, badIndex int) []WriteCommit {
	t.Helper()

	const missingOid = "0123456789abcdef0123456789abcdef01234567"

	noise := rand.NewChaCha8([32]byte{1})
	commits := make([]WriteCommit, 0, 9)

	for index := range 9 {
		content := make([]byte, size)
		if _, err := noise.Read(content); err != nil {
			t.Fatalf("noise.Read() err = %v", err)
		}

		commit := WriteCommit{
			Author: identityAt(1700000000), Committer: identityAt(1700000000),
			Message: "padding",
			Writes:  []FileWrite{{Path: fmt.Sprintf("pad%d", index), Content: content}},
		}

		if index == badIndex {
			commit.Writes = []FileWrite{{Path: "a.go", Oid: missingOid}}
		}

		commits = append(commits, commit)
	}

	return commits
}

func TestWriterWriteMidStreamFailureNeverMovesRef(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		paddingSize int
		isPreExists bool
		isCancelled bool
	}{
		{name: "missing oid within the pipe buffer, ref absent", paddingSize: 1 << 10},
		{name: "missing oid beyond the pipe buffer, ref absent", paddingSize: 400 << 10},
		{
			name: "missing oid within the pipe buffer, ref kept", paddingSize: 1 << 10,
			isPreExists: true,
		},
		{
			name: "missing oid beyond the pipe buffer, ref kept", paddingSize: 400 << 10,
			isPreExists: true,
		},
		{name: "cancelled mid-stream, ref absent", paddingSize: 4 << 20, isCancelled: true},
		{
			name: "cancelled mid-stream, ref kept", paddingSize: 4 << 20, isPreExists: true,
			isCancelled: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := newTestRepo(t)

			oldTip := ""
			if testCase.isPreExists {
				oldTip = seedTip(t, dir)
			}

			badIndex := 4
			if testCase.isCancelled {
				badIndex = -1
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if testCase.isCancelled {
				go cancelOnceImporting(ctx, cancel, dir)
			}

			commits := midStreamCommits(t, testCase.paddingSize, badIndex)

			_, err := NewWriter(dir).Write(ctx, "refs/heads/out", commits)
			if err == nil || errors.Is(err, context.Canceled) != testCase.isCancelled {
				t.Fatalf("Write() err = %v, want failure, cancelled %v", err, testCase.isCancelled)
			}

			got, revErr := revParse(t.Context(), dir, "refs/heads/out")
			if got != oldTip || (oldTip == "") != (revErr != nil) {
				t.Fatalf("ref = %q, %v, want %q", got, revErr, oldTip)
			}
		})
	}
}

func seedTip(t *testing.T, dir string) string {
	t.Helper()

	tip, err := NewWriter(dir).Write(t.Context(), "refs/heads/out", twoCommitFixture()[:1])
	if err != nil {
		t.Fatalf("seed Write() err = %v", err)
	}

	return tip
}

func cancelOnceImporting(ctx context.Context, cancel context.CancelFunc, dir string) {
	pattern := filepath.Join(dir, ".git", "objects", "pack", "tmp_pack_*")

	for ctx.Err() == nil {
		if found, _ := filepath.Glob(pattern); len(found) > 0 {
			cancel()

			return
		}

		runtime.Gosched()
	}
}
