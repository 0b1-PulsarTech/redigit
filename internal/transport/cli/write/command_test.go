package write

import (
	"errors"
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	base := []string{"--repo", "r", "--manifest", "m", "--ref", "refs/heads/x"}
	with := func(extra ...string) []string { return slices.Concat(extra, base) }

	testCases := []struct {
		name    string
		args    []string
		want    options
		wantErr error
	}{
		{
			"every flag", with(
				"--date-ref", "d", "--subjects", "s", "--blob-dir", "b", "--config", "c",
				"--verify", "v", "--author-name", "n", "--author-email", "e", "--allow-draft",
			),
			options{
				repo: "r", manifest: "m", ref: "refs/heads/x", dateRef: "d", subjects: "s",
				blobDir: "b", configPath: "c", verify: "v", authorName: "n", authorEmail: "e",
				allowDraft: true,
			},
			nil,
		},
		{"unknown flag", with("--nope"), options{}, ErrUsage},
		{"missing repo", []string{"--manifest", "m", "--ref", "x"}, options{}, ErrUsage},
		{"missing manifest", []string{"--repo", "r", "--ref", "x"}, options{}, ErrUsage},
		{"missing ref", []string{"--repo", "r", "--manifest", "m"}, options{}, ErrUsage},
		{"positional argument", with("extra"), options{}, ErrUsage},
		{"flag without a value", []string{"--repo"}, options{}, ErrUsage},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parse(testCase.args)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("parse() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("parse() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestParseRefusesBadRefs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ref     string
		wantErr error
	}{
		{"branch ref", "refs/heads/x", nil},
		{"not under refs", "heads/x", ErrUsage},
		{"space", "refs/heads/a b", ErrUsage},
		{"newline", "refs/heads/a\nb", ErrUsage},
		{"control character", "refs/heads/a\x01b", ErrUsage},
		{"delete character", "refs/heads/a\x7fb", ErrUsage},
		{"double dot", "refs/heads/a..b", ErrUsage},
		{"tilde", "refs/heads/a~1", ErrUsage},
		{"caret", "refs/heads/a^", ErrUsage},
		{"colon", "refs/heads/a:b", ErrUsage},
		{"question mark", "refs/heads/a?", ErrUsage},
		{"star", "refs/heads/a*", ErrUsage},
		{"bracket", "refs/heads/a[b", ErrUsage},
		{"backslash", "refs/heads/a\\b", ErrUsage},
		{"reflog syntax", "refs/heads/a@{1}", ErrUsage},
		{"lock suffix", "refs/heads/a.lock", ErrUsage},
		{"trailing slash", "refs/heads/a/", ErrUsage},
		{"trailing dot", "refs/heads/a.", ErrUsage},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := parse([]string{"--repo", "r", "--manifest", "m", "--ref", testCase.ref})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("parse() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
