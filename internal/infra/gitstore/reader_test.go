package gitstore

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

func TestParseHistory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		out  []byte
		want []entities.SourceTouch
	}{
		{"no commits at all", nil, nil},
		{
			"one commit, one path",
			[]byte("\x01aaa111\na.go\x00"),
			[]entities.SourceTouch{{Position: 0, Touched: []string{"a.go"}}},
		},
		{
			"three commits in reverse-chronological git order",
			[]byte(
				"\x01sha1\na.go\x00sub dir/b.go\x00\x01sha2\na.go\x00c.go\x00\x01sha3\nsub dir/b.go\x00",
			),
			[]entities.SourceTouch{
				{Position: 0, Touched: []string{"a.go", "sub dir/b.go"}},
				{Position: 1, Touched: []string{"a.go", "c.go"}},
				{Position: 2, Touched: []string{"sub dir/b.go"}},
			},
		},
		{
			"a commit with no touched paths still occupies a position",
			[]byte("\x01sha1\n"),
			[]entities.SourceTouch{{Position: 0, Touched: nil}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := parseHistory(testCase.out)
			if len(got) != len(testCase.want) {
				t.Fatalf("parseHistory() = %+v, want %+v", got, testCase.want)
			}

			for i, touch := range got {
				want := testCase.want[i]
				if touch.Position != want.Position {
					t.Fatalf(
						"parseHistory()[%d].Position = %d, want %d",
						i,
						touch.Position,
						want.Position,
					)
				}

				if len(touch.Touched) != len(want.Touched) {
					t.Fatalf(
						"parseHistory()[%d].Touched = %v, want %v",
						i,
						touch.Touched,
						want.Touched,
					)
				}

				for j, path := range touch.Touched {
					if path != want.Touched[j] {
						t.Fatalf(
							"parseHistory()[%d].Touched[%d] = %q, want %q",
							i,
							j,
							path,
							want.Touched[j],
						)
					}
				}
			}
		})
	}
}

func TestZeroReaderFailsTyped(t *testing.T) {
	t.Parallel()

	var zero Reader

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "read", call: func() error {
			_, err := zero.Read(t.Context(), "a.go")

			return err
		}},
		{name: "read at", call: func() error {
			_, err := zero.ReadAt(t.Context(), "main", "a.go")

			return err
		}},
		{name: "read blob", call: func() error {
			_, err := zero.ReadBlob(t.Context(), "abc")

			return err
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := testCase.call(); !errors.Is(err, batch.ErrClosed) {
				t.Fatalf("err = %v, want ErrClosed", err)
			}
		})
	}

	if err := zero.Close(); err != nil {
		t.Fatalf("Close() on a zero reader = %v", err)
	}
}

func TestReaderOneShotWithoutGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	reader := NewReader(t.TempDir(), "main")
	t.Cleanup(func() { _ = reader.Close() })

	_, err := reader.ReadAt(t.Context(), "main", "odd\nname")
	if !errors.Is(err, batch.ErrProcess) || errors.Is(err, batch.ErrMissing) ||
		errors.Is(err, ErrAbsentAtRevision) {
		t.Fatalf("ReadAt() err = %v, want ErrProcess only", err)
	}
}

func TestReadOnceRefusesAnOversizeBlob(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell to stand in for git")
	}

	script := "#!/bin/sh\nhead -c 100000 /dev/zero\n"
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	reader := NewReader(t.TempDir(), "main")
	t.Cleanup(func() { _ = reader.Close() })

	testCases := []struct {
		name    string
		limit   int64
		wantLen int
		wantErr error
	}{
		{name: "over the cap", limit: 1000, wantErr: batch.ErrTooLarge},
		{name: "one byte under the output", limit: 99999, wantErr: batch.ErrTooLarge},
		{name: "exactly the output", limit: 100000, wantLen: 100000},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := reader.readOnce(t.Context(), "odd\nname", testCase.limit)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("readOnce() err = %v, want %v", err, testCase.wantErr)
			}

			if errors.Is(err, batch.ErrProcess) || len(got) != testCase.wantLen {
				t.Fatalf("readOnce() = %d bytes, err %v; want %d bytes, no ErrProcess",
					len(got), err, testCase.wantLen)
			}
		})
	}
}

func exitedWith(t testing.TB, script string) error {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}

	err := exec.CommandContext(t.Context(), "sh", "-c", script).Run()
	if _, isExit := errors.AsType[*exec.ExitError](err); !isExit {
		t.Fatalf("script %q did not exit non-zero: %v", script, err)
	}

	return fmt.Errorf("run script: %w", err)
}

func TestClassifyCatFile(t *testing.T) {
	t.Parallel()

	failed := exitedWith(t, "exit 128")
	signaled := exitedWith(t, "kill -KILL $$")

	testCases := []struct {
		name       string
		cause      error
		stderr     string
		want       error
		wantNotErr error
	}{
		{
			name: "path absent from the tree", cause: failed,
			stderr: "fatal: path 'a.go' does not exist in 'HEAD'\n", want: batch.ErrMissing,
		},
		{
			name: "path with a line break", cause: failed,
			stderr: "fatal: path 'a\nb.go' does not exist in 'HEAD'\n", want: batch.ErrMissing,
		},
		{
			name: "path on disk but absent from the tree", cause: failed,
			stderr: "fatal: path 'a.go' exists on disk, but not in 'HEAD'\n",
			want:   batch.ErrMissing,
		},
		{
			name: "missing path naming an ambiguity", cause: failed,
			stderr: "fatal: path 'x is ambiguous' does not exist in 'HEAD'\n",
			want:   batch.ErrMissing, wantNotErr: batch.ErrAmbiguous,
		},
		{
			name: "missing path naming a bad file", cause: failed,
			stderr: "fatal: path 'a bad file' does not exist in 'HEAD'\n",
			want:   batch.ErrMissing, wantNotErr: batch.ErrNotBlob,
		},
		{
			name: "unknown object name", cause: failed,
			stderr: "fatal: Not a valid object name nope\n", want: batch.ErrMissing,
		},
		{
			name: "ambiguous short sha", cause: failed,
			stderr: "error: short object ID abc is ambiguous\nhint: x\n" +
				"fatal: Not a valid object name abc\n",
			want: batch.ErrAmbiguous, wantNotErr: batch.ErrMissing,
		},
		{
			name: "not a blob", cause: failed,
			stderr: "fatal: git cat-file: bad file\n", want: batch.ErrNotBlob,
		},
		{
			name: "missing object directory", cause: failed,
			stderr: "error: object directory /x does not exist; check alternates\n",
			want:   batch.ErrProcess, wantNotErr: batch.ErrMissing,
		},
		{
			name: "unrelated failure", cause: failed,
			stderr: "fatal: not a git repository\n", want: batch.ErrProcess,
		},
		{
			name: "killed by a signal", cause: signaled,
			stderr: "fatal: path 'a.go' does not exist in 'HEAD'\n",
			want:   batch.ErrProcess, wantNotErr: batch.ErrMissing,
		},
		{
			name: "not an exit error", cause: errors.New("no such file"),
			stderr: "fatal: Not a valid object name x\n",
			want:   batch.ErrProcess, wantNotErr: batch.ErrMissing,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertCatFileVerdict(
				t, testCase.cause, testCase.stderr, testCase.want, testCase.wantNotErr,
			)
		})
	}
}

func assertCatFileVerdict(t testing.TB, cause error, stderr string, want, wantNotErr error) {
	t.Helper()

	cmdErr := &commandError{Args: []string{"cat-file"}, Stderr: stderr, cause: cause}

	got := classifyCatFile(cmdErr)
	if !errors.Is(got, want) {
		t.Fatalf("classifyCatFile() = %v, want %v", got, want)
	}

	if wantNotErr != nil && errors.Is(got, wantNotErr) {
		t.Fatalf("classifyCatFile() = %v, must not be %v", got, wantNotErr)
	}
}
