package gitstore

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestSplitNUL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		out  []byte
		want []string
	}{
		{"empty input", nil, nil},
		{"only the trailing NUL", []byte{0}, nil},
		{"one entry", []byte("a.go\x00"), []string{"a.go"}},
		{
			"several entries",
			[]byte("a.go\x00sub dir/b.go\x00c.go\x00"),
			[]string{"a.go", "sub dir/b.go", "c.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := splitNUL(testCase.out)
			if len(got) != len(testCase.want) {
				t.Fatalf("splitNUL() = %v, want %v", got, testCase.want)
			}

			for i, entry := range got {
				if entry != testCase.want[i] {
					t.Fatalf("splitNUL()[%d] = %q, want %q", i, entry, testCase.want[i])
				}
			}
		})
	}
}

type errCloser struct{ err error }

func (closer errCloser) Close() error { return closer.err }

func TestCloseToCancel(t *testing.T) {
	t.Parallel()

	errOther := errors.New("broken pipe")

	testCases := []struct {
		name    string
		closeIn error
		wantErr error
	}{
		{"open stdin closes cleanly", nil, nil},
		{"already closed stdin is a finished process", os.ErrClosed, os.ErrProcessDone},
		{
			"wrapped already closed",
			fmt.Errorf("close: %w", os.ErrClosed),
			os.ErrProcessDone,
		},
		{"another failure passes through", errOther, errOther},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := closeToCancel(errCloser{err: testCase.closeIn})
			if !errors.Is(err, testCase.wantErr) || (err == nil) != (testCase.wantErr == nil) {
				t.Fatalf("closeToCancel() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestCommandError(t *testing.T) {
	t.Parallel()

	cause := errors.New("exit status 128")

	testCases := []struct {
		name string
		err  *commandError
		want string
	}{
		{
			"names the failing subcommand",
			&commandError{Args: []string{"log", "--reverse"}, cause: cause},
			"run git log: exit status 128",
		},
		{
			"falls back to git with no args",
			&commandError{cause: cause},
			"run git git: exit status 128",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.err.Error(); got != testCase.want {
				t.Fatalf("Error() = %q, want %q", got, testCase.want)
			}

			if !errors.Is(testCase.err, cause) {
				t.Fatalf("commandError does not unwrap to its cause")
			}
		})
	}
}

func TestResolveCommitRefusesAnOption(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		rev  string
	}{
		{"a long option", "--all"},
		{"a short option", "-n1"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ResolveCommit(t.Context(), t.TempDir(), testCase.rev); !errors.Is(
				err,
				ErrBadRev,
			) {
				t.Fatalf("ResolveCommit() err = %v, want ErrBadRev", err)
			}
		})
	}
}
