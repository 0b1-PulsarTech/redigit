package batch

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

func TestParseAuthorDates(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		out     string
		want    []rewriting.Stamp
		wantErr error
	}{
		{
			"two zones",
			"200 -0300\n100 +0100\n",
			[]rewriting.Stamp{{Unix: 200, Zone: "-0300"}, {Unix: 100, Zone: "+0100"}},
			nil,
		},
		{
			"blank lines are skipped",
			"\n100 +0000\n\n",
			[]rewriting.Stamp{{Unix: 100, Zone: "+0000"}},
			nil,
		},
		{"empty output", "", nil, nil},
		{"missing zone", "100\n", nil, ErrMalformedLog},
		{"bad stamp", "abc +0000\n", nil, ErrMalformedLog},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseAuthorDates([]byte(testCase.out))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("parseAuthorDates() err = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("parseAuthorDates() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestAuthorDatesIgnoresSignatureConfig(t *testing.T) {
	state := useFakeGit(t, "argv", "100 +0000\n")

	if _, err := AuthorDates(t.Context(), t.TempDir(), "HEAD"); err != nil {
		t.Fatalf("AuthorDates() err = %v", err)
	}

	argv, err := os.ReadFile(filepath.Join(state, "argv"))
	if err != nil {
		t.Fatalf("read recorded argv: %v", err)
	}

	if !slices.Contains(strings.Split(string(argv), "\n"), "--no-show-signature") {
		t.Fatalf("git was run with %q, want --no-show-signature", argv)
	}
}

func TestAuthorDates(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	gitIn(t, repo, "commit", "-q", "--allow-empty", "--date=1700000000 +0200", "-m", "first")
	gitIn(t, repo, "commit", "-q", "--allow-empty", "--date=1700000500 -0300", "-m", "second")

	if err := os.WriteFile(filepath.Join(repo, "main"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write path named like the branch: %v", err)
	}

	testCases := []struct {
		name    string
		ref     string
		want    []rewriting.Stamp
		wantErr error
	}{
		{
			"zones are kept, newest first", "HEAD",
			[]rewriting.Stamp{{Unix: 1700000500, Zone: "-0300"}, {Unix: 1700000000, Zone: "+0200"}},
			nil,
		},
		{
			"a ref that is also a top-level path", "main",
			[]rewriting.Stamp{{Unix: 1700000500, Zone: "-0300"}, {Unix: 1700000000, Zone: "+0200"}},
			nil,
		},
		{"unknown ref is a process error", "refs/heads/nope", nil, ErrProcess},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := AuthorDates(t.Context(), repo, testCase.ref)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("AuthorDates() err = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("AuthorDates() = %v, want %v", got, testCase.want)
			}
		})
	}
}
