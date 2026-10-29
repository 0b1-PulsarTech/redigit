//go:build integration

package gitstore

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

func TestReaderSnapshot(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{"a.go": []byte("hello\n")})
	commitTestFiles(t, dir, "second", map[string][]byte{
		"b.go": []byte("world\n"),
		"a.go": []byte("hello2\n"),
	})

	r := NewReader(dir, "main")

	snap, err := r.Snapshot(t.Context())
	if err != nil {
		t.Fatalf("Snapshot() err = %v", err)
	}

	wantFiles := []string{"a.go", "b.go"}
	gotFiles := slices.Clone(snap.Files)
	slices.Sort(gotFiles)

	if !slices.Equal(gotFiles, wantFiles) {
		t.Fatalf("Snapshot().Files = %v, want %v", gotFiles, wantFiles)
	}

	if !snap.HasHistory() {
		t.Fatalf("Snapshot().HasHistory() = false, want true")
	}

	if len(snap.History) != 2 {
		t.Fatalf("Snapshot().History has %d positions, want 2", len(snap.History))
	}

	if got := snap.FirstTouch("a.go"); got != 0 {
		t.Fatalf("FirstTouch(a.go) = %d, want 0", got)
	}

	if got := snap.FirstTouch("b.go"); got != 1 {
		t.Fatalf("FirstTouch(b.go) = %d, want 1", got)
	}
}

func TestReaderRead(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{"a.go": []byte("hello\n")})
	commitTestFiles(t, dir, "second", map[string][]byte{"a.go": []byte("hello2\n")})

	r := NewReader(dir, "main")

	got, err := r.Read(t.Context(), "a.go")
	if err != nil {
		t.Fatalf("Read() err = %v", err)
	}

	if string(got) != "hello2\n" {
		t.Fatalf("Read() = %q, want %q", got, "hello2\n")
	}
}

func TestReaderReadMissingPath(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{"a.go": []byte("hello\n")})

	r := NewReader(dir, "main")

	if _, err := r.Read(t.Context(), "missing.go"); err == nil {
		t.Fatalf("Read() of a missing path succeeded, want an error")
	}
}

func TestReaderReadsPathWithLineBreak(t *testing.T) {
	t.Parallel()

	const name = "odd\nname.go"

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{name: []byte("odd\n"), "a.go": []byte("a\n")})

	reader := NewReader(dir, "main")
	t.Cleanup(func() { _ = reader.Close() })

	read, err := reader.Read(t.Context(), name)
	if err != nil || string(read) != "odd\n" {
		t.Fatalf("Read() = %q, %v", read, err)
	}

	readAt, err := reader.ReadAt(t.Context(), "main", name)
	if err != nil || string(readAt) != "odd\n" {
		t.Fatalf("ReadAt() = %q, %v", readAt, err)
	}

	if _, err = reader.ReadAt(t.Context(), "main", "gone\nname.go"); !errors.Is(err, ErrAbsentAtRevision) {
		t.Fatalf("ReadAt(missing) err = %v, want ErrAbsentAtRevision", err)
	}

	kept, err := reader.Read(t.Context(), "a.go")
	if err != nil || string(kept) != "a\n" {
		t.Fatalf("batch read after the one-shot = %q, %v", kept, err)
	}
}

func TestReaderOneShotVerdicts(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{
		"dir\nx/f.go": []byte("f\n"),
		"odd\nname":   []byte("odd\n"),
	})

	reader := NewReader(dir, "main")
	t.Cleanup(func() { _ = reader.Close() })

	testCases := []struct {
		name    string
		rev     string
		path    string
		wantErr error
		notErr  error
	}{
		{name: "path absent at the revision", rev: "main", path: "gone\nname", wantErr: ErrAbsentAtRevision},
		{name: "revision unknown", rev: "nonsense", path: "odd\nname", wantErr: ErrAbsentAtRevision},
		{
			name: "absent path naming an ambiguity", rev: "main", path: "gone is ambiguous\nname",
			wantErr: ErrAbsentAtRevision, notErr: batch.ErrAmbiguous,
		},
		{
			name: "directory is not a blob", rev: "main", path: "dir\nx",
			wantErr: batch.ErrNotBlob, notErr: ErrAbsentAtRevision,
		},
		{
			name: "ref starting with a dash", rev: "-x", path: "odd\nname",
			wantErr: ErrAbsentAtRevision, notErr: batch.ErrProcess,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := reader.ReadAt(t.Context(), testCase.rev, testCase.path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ReadAt() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.notErr != nil && errors.Is(err, testCase.notErr) {
				t.Fatalf("ReadAt() err = %v, must not be %v", err, testCase.notErr)
			}
		})
	}
}

func TestReaderOneShotRepoVanished(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{"odd\nname": []byte("odd\n")})

	reader := NewReader(dir, "main")
	t.Cleanup(func() { _ = reader.Close() })

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove repo: %v", err)
	}

	_, err := reader.ReadAt(t.Context(), "main", "odd\nname")
	if !errors.Is(err, batch.ErrProcess) || errors.Is(err, batch.ErrMissing) {
		t.Fatalf("ReadAt() err = %v, want ErrProcess and not ErrMissing", err)
	}
}
