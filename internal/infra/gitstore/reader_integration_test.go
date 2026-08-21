//go:build integration

package gitstore

import (
	"slices"
	"testing"
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
