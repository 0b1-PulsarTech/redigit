package gitstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

func TestInvariantError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *InvariantError
		want string
	}{
		{
			"one diverging path",
			&InvariantError{Diverging: []string{"a.go"}},
			"final tree invariant failed: 1 paths diverge",
		},
		{
			"several diverging paths",
			&InvariantError{Diverging: []string{"a.go", "b.go"}},
			"final tree invariant failed: 2 paths diverge",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.err.Error(); got != testCase.want {
				t.Fatalf("Error() = %q, want %q", got, testCase.want)
			}

			if !errors.Is(testCase.err, apperr.ErrInvariant) {
				t.Fatalf("InvariantError does not unwrap to apperr.ErrInvariant")
			}
		})
	}
}

func TestCheckRefusesDashRefs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		gotRef  string
		wantRef string
	}{
		{"written ref that looks like an option", "--output=x", "HEAD"},
		{"target ref that looks like an option", "HEAD", "--output=x"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := Check(t.Context(), t.TempDir(), testCase.gotRef, testCase.wantRef)
			if !errors.Is(err, ErrBadRev) {
				t.Fatalf("Check() err = %v, want ErrBadRev", err)
			}
		})
	}
}

func TestBlobFilesReadsEachHandleOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "h"), []byte("content"), 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}

	files, err := openBlobFiles(dir)
	if err != nil {
		t.Fatalf("openBlobFiles() err = %v", err)
	}

	defer files.close()

	ref := rewriting.BlobRef{Path: "a", Mode: "100644", Handle: "h"}

	first, err := files.write(ref)
	if err != nil {
		t.Fatalf("first write() err = %v", err)
	}

	if err = os.Remove(filepath.Join(dir, "h")); err != nil {
		t.Fatalf("remove blob: %v", err)
	}

	ref.Path = "b"

	second, err := files.write(ref)

	isShared := err == nil && len(second.Content) > 0 && &second.Content[0] == &first.Content[0]
	if !isShared {
		t.Fatalf("second write() = %q, %v, want the first read's bytes shared", second.Content, err)
	}
}
