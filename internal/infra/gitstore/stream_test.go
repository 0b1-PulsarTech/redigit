package gitstore

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestFormatIdentity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		id   Identity
		want string
	}{
		{
			"utc offset",
			Identity{
				Name:  "Ada Lovelace",
				Email: "ada@example.com",
				When:  time.Unix(1000000000, 0).UTC(),
			},
			"Ada Lovelace <ada@example.com> 1000000000 +0000",
		},
		{
			"non-utc offset",
			Identity{
				Name:  "Grace Hopper",
				Email: "grace@example.com",
				When:  time.Unix(1000000000, 0).In(time.FixedZone("", -3*3600)),
			},
			"Grace Hopper <grace@example.com> 1000000000 -0300",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := formatIdentity(testCase.id); got != testCase.want {
				t.Fatalf("formatIdentity() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func identityAt(unixSeconds int64) Identity {
	return Identity{
		Name:  "Orig Author",
		Email: "orig@example.com",
		When:  time.Unix(unixSeconds, 0).UTC(),
	}
}

// commitHeader is the fixed commit/mark/author/committer/data-message block
// every encodeCommit test case shares, so a case only has to spell out its
// mark, timestamps, message, and fileop tail.
func commitHeader(mark int, authorTS, committerTS int64, message string) string {
	prefix := ""
	if mark == 1 {
		prefix = "reset refs/heads/out\n"
	}

	return prefix + fmt.Sprintf(
		"commit refs/heads/out\nmark :%d\n"+
			"author Orig Author <orig@example.com> %d +0000\n"+
			"committer Orig Author <orig@example.com> %d +0000\n"+
			"data %d\n%s\n",
		mark, authorTS, committerTS, len(message), message,
	)
}

func runEncodeCommit(t *testing.T, mark, parentMark int, commit WriteCommit) (string, error) {
	t.Helper()

	var buf bytes.Buffer

	if err := validateCommit(commit); err != nil {
		return "", err
	}

	w := bufio.NewWriter(&buf)
	if err := encodeCommit(w, "refs/heads/out", mark, parentMark, commit); err != nil {
		return "", err
	}

	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	return buf.String(), nil
}

func TestEncodeCommitWrites(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		mark       int
		parentMark int
		commit     WriteCommit
		want       string
	}{
		{
			"root commit, ascii message, single write",
			1,
			0,
			WriteCommit{
				Author: identityAt(1000000000), Committer: identityAt(1000000001), Message: "first",
				Writes: []FileWrite{{Path: "a.go", Content: []byte("hello\n")}},
			},
			commitHeader(
				1,
				1000000000,
				1000000001,
				"first",
			) + "M 100644 inline a.go\ndata 6\nhello\n\n",
		},
		{
			"non-root commit carries a from line",
			2,
			1,
			WriteCommit{
				Author: identityAt(
					1000000010,
				),
				Committer: identityAt(1000000011),
				Message:   "second",
				Writes:    []FileWrite{{Path: "b.go", Content: []byte("x")}},
			},
			commitHeader(
				2,
				1000000010,
				1000000011,
				"second",
			) + "from :1\nM 100644 inline b.go\ndata 1\nx\n",
		},
		{
			"space path is quoted on the M line",
			1,
			0,
			WriteCommit{
				Author: identityAt(1000000000), Committer: identityAt(1000000000), Message: "m",
				Writes: []FileWrite{{Path: "dir with space/b.go", Content: []byte("x")}},
			},
			commitHeader(
				1,
				1000000000,
				1000000000,
				"m",
			) + `M 100644 inline "dir with space/b.go"` + "\ndata 1\nx\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := runEncodeCommit(t, testCase.mark, testCase.parentMark, testCase.commit)
			if err != nil {
				t.Fatalf("encodeCommit() unexpected err = %v", err)
			}

			if got != testCase.want {
				t.Fatalf("encodeCommit() =\n%q\nwant\n%q", got, testCase.want)
			}
		})
	}
}

func TestEncodeCommitContentEdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		commit WriteCommit
		want   string
	}{
		{
			// "café" is 4 runes but 5 bytes: 'c', 'a', 'f' plus the 2-byte 'é'.
			"multi-byte content: data count is encoded bytes, not runes",
			WriteCommit{
				Author: identityAt(1000000000), Committer: identityAt(1000000000), Message: "m",
				Writes: []FileWrite{{Path: "a.go", Content: []byte("café")}},
			},
			commitHeader(1, 1000000000, 1000000000, "m") + "M 100644 inline a.go\ndata 5\ncafé\n",
		},
		{
			"empty content is a valid zero-length blob",
			WriteCommit{
				Author: identityAt(1000000000), Committer: identityAt(1000000000), Message: "m",
				Writes: []FileWrite{{Path: "empty.go", Content: []byte{}}},
			},
			commitHeader(1, 1000000000, 1000000000, "m") + "M 100644 inline empty.go\ndata 0\n\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := runEncodeCommit(t, 1, 0, testCase.commit)
			if err != nil {
				t.Fatalf("encodeCommit() unexpected err = %v", err)
			}

			if got != testCase.want {
				t.Fatalf("encodeCommit() =\n%q\nwant\n%q", got, testCase.want)
			}
		})
	}
}

func TestEncodeCommitDeletesAndConflicts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		commit  WriteCommit
		want    string
		wantErr error
	}{
		{
			"delete uses D, never an empty M",
			WriteCommit{
				Author: identityAt(1000000000), Committer: identityAt(1000000000), Message: "m",
				Deletes: []string{"dead.go"},
			},
			commitHeader(1, 1000000000, 1000000000, "m") + "D dead.go\n",
			nil,
		},
		{
			"same path written and deleted is rejected",
			WriteCommit{
				Author: identityAt(1000000000), Committer: identityAt(1000000000), Message: "m",
				Writes:  []FileWrite{{Path: "a.go", Content: []byte("x")}},
				Deletes: []string{"a.go"},
			},
			"",
			errPathConflict,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := runEncodeCommit(t, 1, 0, testCase.commit)

			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("encodeCommit() err = %v, want %v", err, testCase.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("encodeCommit() unexpected err = %v", err)
			}

			if got != testCase.want {
				t.Fatalf("encodeCommit() =\n%q\nwant\n%q", got, testCase.want)
			}
		})
	}
}

func TestPathConflictError(t *testing.T) {
	t.Parallel()

	err := &PathConflictError{Path: "a.go"}
	if !errors.Is(err, errPathConflict) {
		t.Fatalf("PathConflictError does not unwrap to errPathConflict")
	}

	if err.Error() == "" {
		t.Fatalf("PathConflictError.Error() is empty")
	}
}

func TestEncodeCommitDataref(t *testing.T) {
	t.Parallel()

	const oid = "0123456789abcdef0123456789abcdef01234567"

	testCases := []struct {
		name  string
		write FileWrite
		want  string
	}{
		{
			"executable oid write has no data block",
			FileWrite{Path: "run.sh", Mode: ModeExecutable, Oid: oid},
			"M 100755 " + oid + " run.sh\n",
		},
		{
			"oid wins over content and the path is quoted",
			FileWrite{Path: "a b.go", Content: []byte("x"), Oid: oid},
			"M 100644 " + oid + ` "a b.go"` + "\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := WriteCommit{
				Author: identityAt(1000000000), Committer: identityAt(1000000000), Message: "m",
				Writes: []FileWrite{testCase.write},
			}

			got, err := runEncodeCommit(t, 1, 0, commit)
			if err != nil {
				t.Fatalf("encodeCommit() unexpected err = %v", err)
			}

			if want := commitHeader(1, 1000000000, 1000000000, "m") + testCase.want; got != want {
				t.Fatalf("encodeCommit() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}
