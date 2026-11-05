package blobstore

import (
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

func TestBlobStorePutRefusesUnsupportedModeBeforeWriting(t *testing.T) {
	t.Parallel()

	// repoPath names no real repository and the store never starts a
	// process: an invalid mode is caught before anything is written, git or
	// otherwise, so this needs no git binary at all.
	s := NewBlobStore("/does/not/exist")

	_, err := s.Put(t.Context(), "vendor/lib", "160000", []byte("x"))

	var modeErr *gitstore.UnsupportedModeError
	if !errors.As(err, &modeErr) {
		t.Fatalf("Put() err = %v, want *UnsupportedModeError", err)
	}

	if modeErr.Path != "vendor/lib" {
		t.Fatalf("Path = %q, want %q", modeErr.Path, "vendor/lib")
	}

	if s.cmd != nil {
		t.Fatalf("Put() started a hash-object process before validating the mode")
	}
}

func TestBlobStoreCloseBeforePutIsANoOp(t *testing.T) {
	t.Parallel()

	s := NewBlobStore(t.TempDir())

	if err := s.Close(); err != nil {
		t.Fatalf("Close() err = %v, want nil", err)
	}
}
