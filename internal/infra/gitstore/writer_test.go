package gitstore

import (
	"bytes"
	"errors"
	"testing"
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

func TestStreamCommitsPropagatesEncodeError(t *testing.T) {
	t.Parallel()

	commits := []WriteCommit{{
		Author: identityAt(1000000000), Committer: identityAt(1000000000), Message: "first",
		Writes:  []FileWrite{{Path: "a.go", Content: []byte("x")}},
		Deletes: []string{"a.go"},
	}}

	var buf bytes.Buffer

	err := streamCommits(&buf, "refs/heads/out", commits)
	if !errors.Is(err, errPathConflict) {
		t.Fatalf("streamCommits() err = %v, want %v", err, errPathConflict)
	}
}

func TestFastImportError(t *testing.T) {
	t.Parallel()

	cause := errors.New("exit status 128")
	err := &fastImportError{Stderr: "fatal: bad ref", cause: cause}

	if got, want := err.Error(), "run fast-import: exit status 128"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	if !errors.Is(err, cause) {
		t.Fatalf("fastImportError does not unwrap to its cause")
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
