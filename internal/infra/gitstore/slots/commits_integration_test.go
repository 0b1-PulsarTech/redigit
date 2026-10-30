//go:build integration

package slots

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func gitAt(t *testing.T, dir, stamp string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestCommitsKeepParentsBeforeChildrenUnderClockSkew(t *testing.T) {
	repo := newRepo(t)

	gitAt(t, repo, "@1000 +0000", "commit", "-q", "--allow-empty", "-m", "base")
	gitAt(t, repo, "@2000 +0000", "commit", "-q", "--allow-empty", "-m", "main work")
	gitIn(t, repo, "checkout", "-q", "-b", "side", "HEAD~1")
	gitAt(t, repo, "@500 +0000", "commit", "-q", "--allow-empty", "-m", "side work")
	gitIn(t, repo, "checkout", "-q", "main")
	gitAt(t, repo, "@3000 +0000", "merge", "-q", "--no-ff", "-m", "merge", "side")

	commits, err := NewCommits(filepath.Join(repo, ".git")).List(t.Context(), "main")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	subjects := make([]string, 0, len(commits))
	for _, commit := range commits {
		subjects = append(subjects, commit.Subject)
	}

	if subjects[0] != "base" || subjects[len(subjects)-1] != "merge" ||
		!slices.Contains(subjects, "side work") {
		t.Fatalf("subjects = %v, want base first and the merge last", subjects)
	}

	if slices.Index(subjects, "side work") < slices.Index(subjects, "base") {
		t.Fatalf("subjects = %v, a child precedes its parent", subjects)
	}
}
