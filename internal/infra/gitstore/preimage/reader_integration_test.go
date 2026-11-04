//go:build integration

package preimage

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t testing.TB, dir string, args ...string) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...).
		CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

func commitFile(t testing.TB, dir, message, name, content string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", message)

	return gitIn(t, dir, "rev-parse", "HEAD")
}

func TestReaderGivesTheSubjectAndTheBlobsOfARevision(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		first      string
		repaired   string
		subject    string
		wantBefore string
		wantAfter  string
	}{
		{
			name:       "a fix commit gives its subject and the blobs on both sides of its change",
			first:      "package p\n\nfunc A() int { return 1 }\n",
			repaired:   "package p\n\nfunc A() int { return 2 }\n",
			subject:    "fix(p): repair A",
			wantBefore: "return 1",
			wantAfter:  "return 2",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			gitIn(t, dir, "init", "-q", "-b", "main")
			gitIn(t, dir, "config", "user.email", "tester@example.com")
			gitIn(t, dir, "config", "user.name", "Tester")
			commitFile(t, dir, "feat: add", "a.go", testCase.first)

			fix := commitFile(t, dir, testCase.subject, "a.go", testCase.repaired)

			reader := NewReader(dir)

			t.Cleanup(func() { _ = reader.Close(context.WithoutCancel(t.Context())) })

			got, err := reader.Revision(t.Context(), fix)
			if err != nil {
				t.Fatalf("Revision() = %v", err)
			}

			if got.Subject != testCase.subject || len(got.Files) != 1 {
				t.Fatalf("Revision() = %+v", got)
			}

			change := got.Files[0]

			before, err := reader.Blob(t.Context(), change.OldBlob)
			if err != nil || !strings.Contains(string(before), testCase.wantBefore) {
				t.Fatalf("parent blob = %q, %v", before, err)
			}

			after, err := reader.Blob(t.Context(), change.Blob)
			if err != nil || !strings.Contains(string(after), testCase.wantAfter) {
				t.Fatalf("blob = %q, %v", after, err)
			}
		})
	}
}

func TestReaderFailures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "user.email", "tester@example.com")
	gitIn(t, dir, "config", "user.name", "Tester")
	commitFile(t, dir, "feat: add", "a.go", "package p\n")

	reader := NewReader(dir)

	t.Cleanup(func() { _ = reader.Close(context.WithoutCancel(t.Context())) })

	testCases := []struct {
		name    string
		rev     string
		wantErr string
	}{
		{
			name: "an unknown revision has no subject",
			rev:  "deadbeef", wantErr: "read subject of deadbeef",
		},
		{
			name: "a revision that is no single commit has no diff",
			rev:  "HEAD^@", wantErr: "read changes of HEAD^@",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := reader.Revision(t.Context(), testCase.rev)
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("Revision(%s) error = %v, want %q", testCase.rev, err, testCase.wantErr)
			}
		})
	}

	if _, err := reader.Blob(t.Context(), strings.Repeat("1", 40)); err == nil {
		t.Fatal("Blob() of an unknown object succeeded")
	}
}
