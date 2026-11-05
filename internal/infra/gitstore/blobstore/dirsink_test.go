package blobstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

func TestDirSinkPutRefusesUnsupportedModeBeforeWriting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sink := NewDirSink(dir)

	_, err := sink.Put(t.Context(), "vendor/lib", "160000", []byte("x"))

	var modeErr *gitstore.UnsupportedModeError
	if !errors.As(err, &modeErr) {
		t.Fatalf("Put() err = %v, want *UnsupportedModeError", err)
	}

	if modeErr.Path != "vendor/lib" {
		t.Fatalf("Path = %q, want %q", modeErr.Path, "vendor/lib")
	}
}

func TestDirSinkIsContentAddressed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sink := NewDirSink(dir)

	first, err := sink.Put(t.Context(), "a/lead.go", gitstore.ModeFile, []byte("one declaration"))
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}

	second, err := sink.Put(t.Context(), "a/lead.go", gitstore.ModeFile, []byte("one and two"))
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}

	if first == "" || second == "" {
		t.Fatalf("handles are %q and %q; a caller cannot find a blob without one", first, second)
	}

	if first == second {
		t.Fatal("two different contents at the same path collapsed onto one handle")
	}

	// Both versions must still exist: a later commit's blob may not erase an
	// earlier commit's.
	for _, handle := range []string{first, second} {
		if _, statErr := os.Stat(filepath.Join(dir, handle)); statErr != nil {
			t.Fatalf("blob %s is gone: %v", handle, statErr)
		}
	}

	// Identical content at a different path is one blob, stored once.
	again, err := sink.Put(t.Context(), "b/other.go", gitstore.ModeFile, []byte("one declaration"))
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}

	if again != first {
		t.Fatalf("identical content produced handles %q and %q", first, again)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("the sink holds %d files, want the 2 distinct contents", len(entries))
	}
}
