package batch

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	zeroBlob = "0000000000000000000000000000000000000000"
	blobOne  = "1111111111111111111111111111111111111111"
	blobTwo  = "2222222222222222222222222222222222222222"
)

func TestParseLog(t *testing.T) {
	t.Parallel()

	header := "\x01abc 1700000000\x00"
	added := "\n:000000 100644 " + zeroBlob + " " + blobOne + " A\x00new.go\x00"
	renamed := "\n:100644 100644 " + blobOne + " " + blobTwo + " R087\x00old.go\x00moved.go\x00"

	testCases := []struct {
		name        string
		input       string
		wantCommits int
		wantEntries []Entry
	}{
		{name: "empty stream"},
		{name: "merge with no entries", input: header, wantCommits: 1},
		{
			name: "added", input: header + added, wantCommits: 1,
			wantEntries: []Entry{{
				OldMode: "000000", NewMode: "100644", OldBlob: zeroBlob, NewBlob: blobOne,
				Status: 'A', OldPath: "new.go", NewPath: "new.go",
			}},
		},
		{
			name: "rename carries similarity", input: header + renamed, wantCommits: 1,
			wantEntries: []Entry{{
				OldMode: "100644", NewMode: "100644", OldBlob: blobOne, NewBlob: blobTwo,
				Status: 'R', Similarity: 87, OldPath: "old.go", NewPath: "moved.go",
			}},
		},
		{name: "two commits", input: header + added + "\x01def 1700000001\x00", wantCommits: 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var commits []Commit

			for commit, err := range ParseLog(strings.NewReader(testCase.input)) {
				if err != nil {
					t.Fatalf("ParseLog() err = %v", err)
				}

				commits = append(commits, commit)
			}

			if len(commits) != testCase.wantCommits {
				t.Fatalf("got %d commits, want %d", len(commits), testCase.wantCommits)
			}

			if testCase.wantEntries == nil {
				return
			}

			if len(commits[0].Entries) != 1 || commits[0].Entries[0] != testCase.wantEntries[0] {
				t.Fatalf("entries = %+v, want %+v", commits[0].Entries, testCase.wantEntries)
			}

			if want := time.Unix(1700000000, 0).UTC(); !commits[0].AuthorDate.Equal(want) {
				t.Fatalf("author date = %v, want %v", commits[0].AuthorDate, want)
			}
		})
	}
}

func TestParseLogMalformed(t *testing.T) {
	t.Parallel()

	header := "\x01abc 1700000000\x00"
	added := "\n:000000 100644 " + zeroBlob + " " + blobOne + " A\x00new.go\x00"
	renamed := "\n:100644 100644 " + blobOne + " " + blobTwo + " R087\x00old.go\x00moved.go\x00"

	testCases := []struct {
		name  string
		input string
	}{
		{name: "entry before header", input: added},
		{name: "bad header", input: "\x01abc\x00"},
		{name: "bad date", input: "\x01abc soon\x00"},
		{name: "short record", input: header + "\n:100644 100644 x\x00"},
		{name: "bad similarity", input: header + strings.Replace(renamed, "R087", "Rxx", 1)},
		{name: "truncated path", input: header + "\n:000000 100644 a b A\x00"},
		{name: "missing rename target", input: header + "\n:1 1 a b R100\x00old\x00"},
		{name: "unterminated header", input: "\x01abc 1"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var gotErr error

			for _, err := range ParseLog(strings.NewReader(testCase.input)) {
				if err != nil {
					gotErr = err

					break
				}
			}

			if !errors.Is(gotErr, ErrMalformedLog) {
				t.Fatalf("err = %v, want ErrMalformedLog", gotErr)
			}
		})
	}
}

func TestParseLogStopsWhenCallerBreaks(t *testing.T) {
	t.Parallel()

	input := "\x01a 1\x00\x01b 2\x00\x01c 3\x00"
	seen := 0

	for range ParseLog(strings.NewReader(input)) {
		seen++

		break
	}

	if seen != 1 {
		t.Fatalf("seen = %d, want 1", seen)
	}
}

func TestParseLogHeaderAfterEntries(t *testing.T) {
	t.Parallel()

	input := "\x01abc 1700000000\x00" +
		"\n:000000 100644 " + zeroBlob + " " + blobOne + " A\x00new.go\x00" +
		"\x01def 1700000001\x00" +
		"\n:100644 100644 " + blobOne + " " + blobTwo + " R087\x00old.go\x00moved.go\x00"

	var commits []Commit

	for commit, err := range ParseLog(strings.NewReader(input)) {
		if err != nil {
			t.Fatalf("ParseLog() err = %v", err)
		}

		commits = append(commits, commit)
	}

	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}

	testCases := []struct {
		name     string
		commit   int
		wantOID  string
		wantPath string
	}{
		{name: "first keeps its own entry", commit: 0, wantOID: "abc", wantPath: "new.go"},
		{name: "second takes the next entries", commit: 1, wantOID: "def", wantPath: "moved.go"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := commits[testCase.commit]
			if got.OID != testCase.wantOID || len(got.Entries) != 1 ||
				got.Entries[0].NewPath != testCase.wantPath {
				t.Fatalf("commit = %+v", got)
			}
		})
	}
}
