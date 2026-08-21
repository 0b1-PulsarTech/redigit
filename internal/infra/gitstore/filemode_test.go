package gitstore

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFileModeValid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		mode FileMode
		want bool
	}{
		{"plain file", ModeFile, true},
		{"executable", ModeExecutable, true},
		{"symlink", ModeSymlink, true},
		{"a submodule is not a blob", "160000", false},
		{"a directory is not a blob", "040000", false},
		{"empty is not valid on its own; FileWrite defaults it instead", "", false},
		{"nonsense", "abc", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.mode.Valid(); got != testCase.want {
				t.Fatalf("Valid() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func encodeOne(t *testing.T, write FileWrite) (string, error) {
	t.Helper()

	var buf bytes.Buffer

	w := bufio.NewWriter(&buf)
	when := time.Unix(1000000000, 0).UTC()
	id := Identity{Name: "T", Email: "t@example.com", When: when}

	err := encodeCommit(w, "refs/heads/main", 1, 0, WriteCommit{
		Author:    id,
		Committer: id,
		Message:   "m",
		Writes:    []FileWrite{write},
	})
	if err != nil {
		return "", err
	}

	if flushErr := w.Flush(); flushErr != nil {
		t.Fatalf("flush: %v", flushErr)
	}

	return buf.String(), nil
}

func TestEncodeCommitCarriesFileMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		write   FileWrite
		want    string
		wantErr error
	}{
		{
			"an unset mode defaults to a plain file",
			FileWrite{Path: "a.go", Content: []byte("x")},
			"M 100644 inline a.go",
			nil,
		},
		{
			"an executable keeps its bit, or the rebuilt tree differs from the target",
			FileWrite{Path: "run.sh", Mode: ModeExecutable, Content: []byte("#!/bin/sh\n")},
			"M 100755 inline run.sh",
			nil,
		},
		{
			"a symlink is a blob holding its target",
			FileWrite{Path: "link", Mode: ModeSymlink, Content: []byte("run.sh")},
			"M 120000 inline link",
			nil,
		},
		{
			"a submodule is refused here, naming the path, rather than by a stream parse error naming none",
			FileWrite{Path: "vendor", Mode: "160000", Content: nil},
			"",
			errUnsupportedMode,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := encodeOne(t, testCase.write)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("encodeCommit() = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				return
			}

			if !strings.Contains(got, testCase.want) {
				t.Fatalf("stream does not contain %q:\n%s", testCase.want, got)
			}
		})
	}
}

func TestUnsupportedModeErrorNamesThePath(t *testing.T) {
	t.Parallel()

	_, err := encodeOne(t, FileWrite{Path: "vendor/lib", Mode: "160000"})

	var modeErr *UnsupportedModeError
	if !errors.As(err, &modeErr) {
		t.Fatalf("err = %v, want an *UnsupportedModeError", err)
	}

	if modeErr.Path != "vendor/lib" {
		t.Fatalf("Path = %q, want %q", modeErr.Path, "vendor/lib")
	}

	// The message stays low-cardinality; the path travels as a field so a log
	// site can attach it without destroying grouping.
	if strings.Contains(modeErr.Error(), "vendor/lib") {
		t.Fatalf("message %q carries variable data", modeErr.Error())
	}
}
