package gitstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

func TestStreamCommits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		commits []WriteCommit
		want    string
	}{
		{"no commits", nil, "done\n"},
		{
			"two commits chain by mark",
			[]WriteCommit{
				{
					Author: identityAt(
						1000000000,
					),
					Committer: identityAt(1000000000),
					Message:   "first",
					Writes:    []FileWrite{{Path: "a.go", Content: []byte("x")}},
				},
				{
					Author: identityAt(
						1000000010,
					),
					Committer: identityAt(1000000010),
					Message:   "second",
					Writes:    []FileWrite{{Path: "b.go", Content: []byte("y")}},
				},
			},
			commitHeader(1, 1000000000, 1000000000, "first") + "M 100644 inline a.go\ndata 1\nx\n" +
				commitHeader(
					2,
					1000000010,
					1000000010,
					"second",
				) + "from :1\nM 100644 inline b.go\ndata 1\ny\n" +
				"done\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			if err := streamCommits(&buf, "refs/heads/out", testCase.commits); err != nil {
				t.Fatalf("streamCommits() unexpected err = %v", err)
			}

			if got := buf.String(); got != testCase.want {
				t.Fatalf("streamCommits() =\n%q\nwant\n%q", got, testCase.want)
			}
		})
	}
}

func TestWriteNoCommits(t *testing.T) {
	t.Parallel()

	w := NewWriter(t.TempDir())

	_, err := w.Write(t.Context(), "refs/heads/out", nil)
	if !errors.Is(err, errNoCommits) {
		t.Fatalf("Write() err = %v, want %v", err, errNoCommits)
	}
}

func TestWriteRefusesInvalidCommitBeforeStart(t *testing.T) {
	t.Parallel()

	commits := []WriteCommit{{
		Author: identityAt(1000000000), Committer: identityAt(1000000000), Message: "first",
		Writes:  []FileWrite{{Path: "a.go", Content: []byte("x")}},
		Deletes: []string{"a.go"},
	}}

	_, err := NewWriter(t.TempDir()).Write(t.Context(), "refs/heads/out", commits)
	if !errors.Is(err, errPathConflict) {
		t.Fatalf("Write() err = %v, want %v", err, errPathConflict)
	}
}

func TestFastImportError(t *testing.T) {
	t.Parallel()

	cause := errors.New("exit status 128")

	testCases := []struct {
		name   string
		stderr string
		want   string
	}{
		{
			"stderr is folded into the message",
			"fatal: bad ref",
			"run fast-import: exit status 128: fatal: bad ref",
		},
		{"no stderr leaves the bare cause", "", "run fast-import: exit status 128"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := &fastImportError{Stderr: testCase.stderr, cause: cause}

			if got := err.Error(); got != testCase.want {
				t.Fatalf("Error() = %q, want %q", got, testCase.want)
			}

			if !errors.Is(err, cause) {
				t.Fatalf("fastImportError does not unwrap to its cause")
			}
		})
	}
}

type failingCloser struct {
	inner io.Closer
	err   error
}

func (closer failingCloser) Close() error {
	_ = closer.inner.Close()

	return closer.err
}

func TestFinishImportWaitsAfterAFailedClose(t *testing.T) {
	t.Parallel()

	errClose := errors.New("close failed")
	succeeds := []string{"hash-object", "--stdin"}
	fails := []string{"no-such-subcommand"}

	testCases := []struct {
		name            string
		args            []string
		closeErr        error
		wantErr         error
		wantExitFailure bool
	}{
		{"a failed close still reaps the process", succeeds, errClose, errClose, false},
		{"a clean close and clean exit", succeeds, nil, nil, false},
		{"both failures are joined", fails, errClose, errClose, true},
		{"a clean close and a failed exit", fails, nil, nil, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.CommandContext(t.Context(), "git", testCase.args...)
			cmd.Dir = t.TempDir()

			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatalf("StdinPipe() err = %v", err)
			}

			if err = cmd.Start(); err != nil {
				t.Fatalf("Start() err = %v", err)
			}

			var stderr batch.TailBuffer

			closer := failingCloser{inner: stdin, err: testCase.closeErr}
			err = finishImport(closer, cmd, &stderr)

			if cmd.ProcessState == nil {
				t.Fatalf("finishImport() returned without waiting for the process")
			}

			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Fatalf("finishImport() err = %v, want %v", err, testCase.wantErr)
			}

			_, isExitFailure := errors.AsType[*fastImportError](err)
			if want := testCase.wantExitFailure; isExitFailure != want {
				t.Fatalf("finishImport() err = %v, want exit failure %v", err, want)
			}

			if testCase.wantErr == nil && !testCase.wantExitFailure && err != nil {
				t.Fatalf("finishImport() err = %v, want nil", err)
			}
		})
	}
}

func TestFinishImportCountsACleanExitAfterALateCancel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		isCancelled bool
	}{
		{"cancel seen before the stream closes", true},
		{"no cancel at all", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			seen := make(chan struct{})
			cmd := exec.CommandContext(ctx, "git", "hash-object", "--stdin")
			cmd.Dir = t.TempDir()
			cmd.Cancel = func() error {
				close(seen)

				return nil
			}

			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatalf("StdinPipe() err = %v", err)
			}

			if err = cmd.Start(); err != nil {
				t.Fatalf("Start() err = %v", err)
			}

			if testCase.isCancelled {
				cancel()
				<-seen
			}

			var stderr batch.TailBuffer
			if err = finishImport(stdin, cmd, &stderr); err != nil {
				t.Fatalf("finishImport() err = %v, want nil for a process that exited 0", err)
			}
		})
	}
}

func TestWriteRefusesABadRefBeforeStart(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		ref  string
	}{
		{"outside refs", "main"},
		{"holds a space", "refs/heads/a b"},
		{"holds a newline", "refs/heads/a\ncommit refs/heads/b"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commits := []WriteCommit{{
				Author: identityAt(1), Committer: identityAt(1), Message: "m",
				Writes: []FileWrite{{Path: "a.go", Content: []byte("x")}},
			}}

			_, err := NewWriter(t.TempDir()).Write(t.Context(), testCase.ref, commits)
			if !errors.Is(err, ErrBadRef) {
				t.Fatalf("Write() err = %v, want ErrBadRef", err)
			}
		})
	}
}

func TestFastImportCmdEndsOnCancelWithoutAKill(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := runGit(t.Context(), repo, "init", "-q"); err != nil {
		t.Fatalf("init: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cmd, stdin, err := fastImportCmd(ctx, repo)
	if err != nil {
		t.Fatalf("fastImportCmd() err = %v", err)
	}

	if err = cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err = io.WriteString(stdin, "blob\nmark :1\n"); err != nil {
		t.Fatalf("write partial stream: %v", err)
	}

	cancel()

	exitErr, isExit := errors.AsType[*exec.ExitError](cmd.Wait())
	if !isExit || exitErr.ExitCode() <= 0 {
		t.Fatalf("Wait() = %v, want fast-import to exit by itself on EOF, not be killed", exitErr)
	}
}
