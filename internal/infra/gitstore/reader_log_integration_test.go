//go:build integration

package gitstore

import (
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

func TestReaderChangesAndBlobs(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{"a.go": []byte("one\ntwo\nthree\nfour\n")})
	runTestGit(t, dir, "mv", "a.go", "b.go")
	runTestGit(t, dir, "commit", "-q", "-m", "second")
	runTestGit(t, dir, "rm", "-q", "b.go")
	runTestGit(t, dir, "commit", "-q", "-m", "third")

	reader := NewReader(dir, "main")

	t.Cleanup(func() { _ = reader.Close() })

	var changes []indexing.RevisionChange

	for change, err := range reader.Changes(t.Context()) {
		if err != nil {
			t.Fatalf("Changes() err = %v", err)
		}

		changes = append(changes, change)
	}

	if len(changes) != 3 {
		t.Fatalf("got %d changes, want 3", len(changes))
	}

	testCases := []struct {
		name        string
		change      int
		wantPath    string
		wantDeleted bool
	}{
		{name: "added", change: 0, wantPath: "a.go"},
		{name: "rename reports the new path", change: 1, wantPath: "b.go"},
		{name: "delete", change: 2, wantPath: "b.go", wantDeleted: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			files := changes[testCase.change].Files
			if len(files) != 1 || files[0].Path != testCase.wantPath ||
				files[0].IsDeleted != testCase.wantDeleted {
				t.Fatalf("files = %+v", files)
			}

			if testCase.wantDeleted {
				return
			}

			got, err := reader.ReadBlob(t.Context(), files[0].Blob)
			if err != nil || string(got) != "one\ntwo\nthree\nfour\n" {
				t.Fatalf("ReadBlob() = %q, %v", got, err)
			}
		})
	}
}

func TestReaderReadFailures(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{"a.go": []byte("one\n")})

	reader := NewReader(dir, "main")

	t.Cleanup(func() { _ = reader.Close() })

	_, err := reader.ReadAt(t.Context(), "main", "gone.go")
	if !errors.Is(err, ErrAbsentAtRevision) {
		t.Fatalf("ReadAt(missing) err = %v, want ErrAbsentAtRevision", err)
	}

	_, err = reader.ReadBlob(t.Context(), strings.Repeat("0", 40))
	if !errors.Is(err, indexing.ErrBlobMissing) {
		t.Fatalf("ReadBlob(missing) err = %v, want ErrBlobMissing", err)
	}

	if err = reader.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	if _, err = reader.Read(t.Context(), "a.go"); !errors.Is(err, batch.ErrClosed) {
		t.Fatalf("Read() after Close err = %v, want ErrClosed", err)
	}
}

func TestReaderReadBlobTreatsNonBlobAsUnreadable(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "first", map[string][]byte{"a.go": []byte("one\n")})

	commitOID, err := revParse(t.Context(), dir, "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}

	reader := NewReader(dir, "main")

	t.Cleanup(func() { _ = reader.Close() })

	_, err = reader.ReadBlob(t.Context(), commitOID)
	if !errors.Is(err, indexing.ErrBlobMissing) || !errors.Is(err, batch.ErrNotBlob) {
		t.Fatalf("ReadBlob(commit) err = %v, want ErrBlobMissing and ErrNotBlob", err)
	}

	if got, err := reader.ReadBlob(t.Context(), "HEAD:a.go"); err != nil || string(got) != "one\n" {
		t.Fatalf("read after the refused object = %q, %v", got, err)
	}
}
