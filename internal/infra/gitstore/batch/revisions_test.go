package batch

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
)

func TestRevisionOf(t *testing.T) {
	t.Parallel()

	header := "\x01abc 1700000000\x00"
	added := "\n:000000 100644 " + zeroBlob + " " + blobOne + " A\x00new.go\x00"
	renamed := "\n:100644 100644 " + blobOne + " " + blobTwo + " R087\x00old.go\x00moved.go\x00"
	deleted := "\n:100644 000000 " + blobTwo + " " + zeroBlob + " D\x00moved.go\x00"

	testCases := []struct {
		name  string
		input string
		want  []lineage.Change
	}{
		{"a merge holds no change", header, []lineage.Change{}},
		{"an add", header + added, []lineage.Change{{
			Path: "new.go", OldPath: "new.go", Blob: blobOne, OldBlob: zeroBlob, Status: 'A',
		}}},
		{"a rename keeps both sides and its score", header + renamed, []lineage.Change{{
			Path: "moved.go", OldPath: "old.go", Blob: blobTwo, OldBlob: blobOne,
			Status: 'R', Similarity: 87,
		}}},
		{"a delete", header + deleted, []lineage.Change{{
			Path: "moved.go", OldPath: "moved.go", Blob: zeroBlob, OldBlob: blobTwo,
			Status: lineage.StatusDeleted,
		}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for commit, err := range ParseLog(strings.NewReader(testCase.input)) {
				if err != nil {
					t.Fatalf("ParseLog() err = %v", err)
				}

				got := revisionOf(commit)
				if got.Rev != "abc" || !reflect.DeepEqual(got.Changes, testCase.want) {
					t.Fatalf("revisionOf = %+v, want %v", got, testCase.want)
				}
			}
		})
	}
}

func TestRevisionsReadBlobFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		oid         string
		wantMissing bool
		wantErr     error
	}{
		{"a spec with a line break", "a\nb", false, ErrBadSpec},
		{"an empty spec", "", false, ErrBadSpec},
		{"an object the repo lacks", blobOne, true, ErrMissing},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			revisions := NewRevisions(newRepo(t), "HEAD")
			t.Cleanup(func() { _ = revisions.Close() })

			_, err := revisions.ReadBlob(t.Context(), testCase.oid)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ReadBlob() = %v, want %v", err, testCase.wantErr)
			}

			if errors.Is(err, lineage.ErrBlobMissing) != testCase.wantMissing {
				t.Fatalf("ReadBlob() = %v, missing = %v", err, testCase.wantMissing)
			}
		})
	}
}

func TestRevisionsReadBlobVerdicts(t *testing.T) {
	testCases := []struct {
		name        string
		reply       string
		wantErr     error
		wantMissing bool
	}{
		{"an object that is not a blob", "abc commit 5\nhello\n", ErrNotBlob, true},
		{"a blob too large to read", "abc blob 99999999999999\n", ErrTooLarge, true},
		{"a reply git should not give", "garbage\n", ErrProcess, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			useFakeGit(t, "reply", testCase.reply)

			revisions := NewRevisions(t.TempDir(), "HEAD")

			t.Cleanup(func() { _ = revisions.Close() })

			_, err := revisions.ReadBlob(t.Context(), blobOne)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ReadBlob() = %v, want %v", err, testCase.wantErr)
			}

			if errors.Is(err, lineage.ErrBlobMissing) != testCase.wantMissing {
				t.Fatalf("ReadBlob() = %v, missing = %v", err, testCase.wantMissing)
			}
		})
	}
}

func TestRevisionsChangesFailsOnABadRef(t *testing.T) {
	t.Parallel()

	revisions := NewRevisions(newRepo(t), "nope")

	t.Cleanup(func() { _ = revisions.Close() })

	var failures []error

	for _, err := range revisions.Changes(t.Context()) {
		failures = append(failures, err)
	}

	if len(failures) != 1 || !errors.Is(failures[0], ErrProcess) ||
		!strings.HasPrefix(failures[0].Error(), "read log: ") {
		t.Fatalf("Changes() errors = %v, want one wrapped ErrProcess", failures)
	}
}

func TestRevisionsChangesStopsEarly(t *testing.T) {
	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "1\n"})
	commitFiles(t, repo, "two", map[string]string{"a.go": "2\n"})

	revisions := NewRevisions(repo, "main")

	t.Cleanup(func() { _ = revisions.Close() })

	seen := 0

	for revision, err := range revisions.Changes(t.Context()) {
		if err != nil || revision.Rev == "" {
			t.Fatalf("Changes() = %+v, %v", revision, err)
		}

		seen++

		break
	}

	if seen != 1 {
		t.Fatalf("seen = %d, want 1", seen)
	}

	if left := gitChildren(t); left != 0 {
		t.Fatalf("%d git processes outlived the stopped iteration", left)
	}
}

func gitChildren(t testing.TB) int {
	t.Helper()

	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Skipf("no /proc: %v", err)
	}

	parent, count := strconv.Itoa(os.Getpid()), 0

	for _, entry := range entries {
		stat, readErr := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if readErr != nil {
			continue
		}

		_, rest, _ := strings.Cut(string(stat), ") ")
		fields := strings.Fields(rest)
		isGit := strings.Contains(string(stat), "(git)")

		if isGit && len(fields) > 1 && fields[1] == parent {
			count++
		}
	}

	return count
}

func TestRevisionsClose(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "alpha\n"})

	revisions := NewRevisions(repo, "main")

	if _, err := revisions.ReadBlob(t.Context(), "HEAD:a.go"); err != nil {
		t.Fatalf("warm-up read: %v", err)
	}

	pid := revisions.stream.proc.cmd.Process.Pid

	for attempt := range 2 {
		if err := revisions.Close(); err != nil {
			t.Fatalf("Close() #%d = %v", attempt+1, err)
		}
	}

	if !isReaped(pid) {
		t.Fatal("cat-file was not reaped by Close")
	}

	_, err := revisions.ReadBlob(t.Context(), "HEAD:a.go")
	if !errors.Is(err, ErrClosed) || errors.Is(err, lineage.ErrBlobMissing) {
		t.Fatalf("ReadBlob() after Close = %v, want ErrClosed", err)
	}
}
