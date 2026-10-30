//go:build unix

package gitstore

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

func TestBlobFilesRefusesWhatIsNotARegularFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		handle  string
		prepare func(t *testing.T, path string)
		wantErr error
	}{
		{
			"fifo", "pipe",
			func(t *testing.T, path string) {
				t.Helper()

				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatalf("mkfifo: %v", err)
				}
			},
			ErrBadHandle,
		},
		{
			"directory", "folder",
			func(t *testing.T, path string) {
				t.Helper()

				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
			ErrBadHandle,
		},
		{
			"regular file", "plain",
			func(t *testing.T, path string) {
				t.Helper()

				if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
			},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			testCase.prepare(t, filepath.Join(dir, testCase.handle))

			files, err := openBlobFiles(dir)
			if err != nil {
				t.Fatalf("openBlobFiles() err = %v", err)
			}

			defer files.close()

			blob := rewriting.BlobRef{Path: "a", Mode: "100644", Handle: testCase.handle}

			_, err = files.write(blob)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("write() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
