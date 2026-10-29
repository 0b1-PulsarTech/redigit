//go:build unix

package rules

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadBoundedRefusesWhatNeverEnds(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path func(t testing.TB) string
	}{
		{"a FIFO", func(t testing.TB) string { return makeFIFO(t) }},
		{"a symlink to a FIFO", func(t testing.TB) string { return linkTo(t, makeFIFO(t)) }},
		{"a device", func(testing.TB) string { return "/dev/zero" }},
		{"a symlink to a device", func(t testing.TB) string { return linkTo(t, "/dev/zero") }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := testCase.path(t)
			done := make(chan error, 1)

			go func() {
				_, err := ReadBounded(path)
				done <- err
			}()

			select {
			case err := <-done:
				if !errors.Is(err, ErrNotRegular) {
					t.Fatalf("ReadBounded() err = %v, want ErrNotRegular", err)
				}

				if want := "not a regular file"; err.Error() != want {
					t.Fatalf("ReadBounded() err = %q, want %q", err, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("ReadBounded() did not return")
			}
		})
	}
}

func makeFIFO(t testing.TB) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "redigit.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	return path
}

func linkTo(t testing.TB, target string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "redigit.json")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	return path
}
