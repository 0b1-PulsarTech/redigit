package gitstore

import (
	"errors"
	"strings"
	"testing"
)

func TestOpenStageRefusesTheScratchNamespace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		ref  string
	}{
		{"scratch ref itself", "refs/redigit/write/1-ab"},
		{"scratch namespace root", "refs/redigit"},
		{"directory under the namespace", "refs/redigit/other"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := openStage(t.Context(), t.TempDir(), testCase.ref)
			if !errors.Is(err, ErrBadRef) {
				t.Fatalf("openStage(%q) err = %v, want ErrBadRef", testCase.ref, err)
			}
		})
	}
}

func TestRefFailure(t *testing.T) {
	t.Parallel()

	errExit := errors.New("exit status 128")

	testCases := []struct {
		name        string
		err         error
		wantMoved   bool
		wantMessage string
	}{
		{
			"ref at another value",
			&commandError{Stderr: "cannot lock ref: is at a but expected b\n", cause: errExit},
			true, "is at a but expected b",
		},
		{
			"ref created meanwhile",
			&commandError{Stderr: "cannot lock ref: reference already exists", cause: errExit},
			true, "reference already exists",
		},
		{
			"ref deleted meanwhile",
			&commandError{Stderr: "cannot lock ref: unable to resolve reference", cause: errExit},
			true, "unable to resolve reference",
		},
		{
			"stranded lock is not a lost race",
			&commandError{Stderr: "Unable to create 'x.lock': File exists.", cause: errExit},
			false, "File exists",
		},
		{"error without git output passes through", errExit, false, "exit status 128"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := refFailure(testCase.err)
			if errors.Is(got, ErrRefMoved) != testCase.wantMoved ||
				!errors.Is(got, errExit) || !strings.Contains(got.Error(), testCase.wantMessage) {
				t.Fatalf(
					"refFailure() = %v, want moved %v and %q",
					got,
					testCase.wantMoved,
					testCase.wantMessage,
				)
			}
		})
	}
}

func TestCheckRef(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ref     string
		wantErr error
	}{
		{"branch", "refs/heads/out", nil},
		{"tag", "refs/tags/v1.0", nil},
		{"empty", "", ErrBadRef},
		{"not under refs", "heads/out", ErrBadRef},
		{"option shaped", "--force", ErrBadRef},
		{"space", "refs/heads/a b", ErrBadRef},
		{"control byte", "refs/heads/a\nb", ErrBadRef},
		{"dot-dot", "refs/heads/a..b", ErrBadRef},
		{"reflog syntax", "refs/heads/a@{1}", ErrBadRef},
		{"lock suffix", "refs/heads/a.lock", ErrBadRef},
		{"trailing slash", "refs/heads/a/", ErrBadRef},
		{"trailing dot", "refs/heads/a.", ErrBadRef},
		{"glob", "refs/heads/*", ErrBadRef},
		{"dot-led component", "refs/heads/.hidden", ErrBadRef},
		{"dot-led middle component", "refs/.x/a", ErrBadRef},
		{"empty component", "refs/heads//a", ErrBadRef},
		{"lock suffix mid-path", "refs/heads/a.lock/b", ErrBadRef},
		{"delete byte", "refs/heads/a\x7fb", ErrBadRef},
		{"tab", "refs/heads/a\tb", ErrBadRef},
		{"lone at component", "refs/heads/@", nil},
		{"c1 control character", "refs/heads/a\u0085b", nil},
		{"non-ascii name", "refs/heads/café", nil},
		{"at inside a name", "refs/heads/a@b", nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := CheckRef(testCase.ref); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("CheckRef(%q) err = %v, want %v", testCase.ref, err, testCase.wantErr)
			}
		})
	}
}
