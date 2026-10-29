//go:build unix

package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

var fakeGitDir string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "fakegit")
	if err != nil {
		fmt.Fprintln(os.Stderr, "make fake git dir:", err)

		return 1
	}

	defer func() { _ = os.RemoveAll(dir) }()

	build := exec.CommandContext(context.Background(), "go", "build", "-o",
		filepath.Join(dir, "git"), "./batch/testdata/fakegit")
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build fake git: %v\n%s", buildErr, out)

		return 1
	}

	fakeGitDir = dir

	return m.Run()
}

func TestReadBlobClassifiesFailures(t *testing.T) {
	testCases := []struct {
		name     string
		mode     string
		reply    string
		wantErr  error
		notErr   error
		wantBlob string
	}{
		{name: "blob", mode: "reply", reply: "abc blob 3\nabc\n", wantBlob: "abc"},
		{
			name: "missing", mode: "reply", reply: "abc missing\n",
			wantErr: indexing.ErrBlobMissing,
		},
		{
			name: "not a blob", mode: "reply", reply: "abc tree 2\nhi\n",
			wantErr: indexing.ErrBlobMissing,
		},
		{
			name: "over the size cap", mode: "oversize",
			wantErr: indexing.ErrBlobMissing, notErr: batch.ErrProcess,
		},
		{
			name: "garbage reply", mode: "reply", reply: "garbage\n",
			wantErr: batch.ErrProcess, notErr: indexing.ErrBlobMissing,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("PATH", fakeGitDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("FAKEGIT_MODE", testCase.mode)
			t.Setenv("FAKEGIT_TEXT", strconv.Quote(testCase.reply))

			reader := NewReader(t.TempDir(), "main")
			t.Cleanup(func() { _ = reader.Close() })

			got, err := reader.ReadBlob(t.Context(), "abc")
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ReadBlob() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.notErr != nil && errors.Is(err, testCase.notErr) {
				t.Fatalf("ReadBlob() err = %v, must not be %v", err, testCase.notErr)
			}

			if string(got) != testCase.wantBlob {
				t.Fatalf("ReadBlob() = %q, want %q", got, testCase.wantBlob)
			}
		})
	}
}
