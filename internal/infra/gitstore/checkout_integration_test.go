//go:build integration

package gitstore

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenStageInABareRepository(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ref     string
		prepare func(t *testing.T, bare string)
		wantErr error
	}{
		{"branch HEAD names", "refs/heads/main", func(*testing.T, string) {}, nil},
		{
			"branch HEAD does not name", "refs/heads/spare",
			func(t *testing.T, bare string) {
				t.Helper()
				runTestGit(t, bare, "branch", "spare")
			}, nil,
		},
		{
			"branch checked out in a linked worktree", "refs/heads/feature",
			func(t *testing.T, bare string) {
				t.Helper()
				runTestGit(t, bare, "worktree", "add", "-q", "-b", "feature",
					filepath.Join(t.TempDir(), "linked"))
			}, ErrRefBusy,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := newTestRepo(t)
			commitTestFiles(t, dir, "seed", map[string][]byte{"a.go": []byte("One\n")})

			bare := filepath.Join(t.TempDir(), "bare.git")
			runTestGit(t, dir, "clone", "-q", "--bare", dir, bare)
			testCase.prepare(t, bare)

			staged, err := openStage(t.Context(), bare, testCase.ref)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("openStage() err = %v, want %v", err, testCase.wantErr)
			}

			wantBase := testCase.wantErr == nil
			if hasBase := staged.Base != ""; hasBase != wantBase {
				t.Fatalf("openStage() base = %q, want a base: %v", staged.Base, wantBase)
			}
		})
	}
}

func TestOpenStageReadsTheTarget(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ref      string
		prepare  func(t *testing.T, dir string)
		wantErr  error
		wantBase bool
	}{
		{
			"absent ref has no base", "refs/heads/out",
			func(*testing.T, string) {}, nil, false,
		},
		{
			"unchecked branch has its tip as base", "refs/heads/spare",
			func(t *testing.T, dir string) {
				t.Helper()
				runTestGit(t, dir, "branch", "spare")
			}, nil, true,
		},
		{
			"branch checked out in the main worktree", "refs/heads/main",
			func(*testing.T, string) {}, ErrRefBusy, false,
		},
		{
			"branch checked out in a linked worktree", "refs/heads/feature",
			func(t *testing.T, dir string) {
				t.Helper()
				runTestGit(t, dir, "worktree", "add", "-q", "-b", "feature",
					filepath.Join(t.TempDir(), "linked"))
			}, ErrRefBusy, false,
		},
		{
			"symbolic ref", "refs/heads/alias",
			func(t *testing.T, dir string) {
				t.Helper()
				runTestGit(t, dir, "symbolic-ref", "refs/heads/alias", "refs/heads/main")
			}, ErrRefBusy, false,
		},
		{
			"ref that would become a directory", "refs/heads/out",
			func(t *testing.T, dir string) {
				t.Helper()
				runTestGit(t, dir, "update-ref", "refs/heads/out/sub", "HEAD")
			}, ErrRefBusy, false,
		},
		{
			"similar name is not below the target", "refs/heads/out",
			func(t *testing.T, dir string) {
				t.Helper()
				runTestGit(t, dir, "update-ref", "refs/heads/outer", "HEAD")
			}, nil, false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := newTestRepo(t)
			commitTestFiles(t, dir, "seed", map[string][]byte{"a.go": []byte("One\n")})
			testCase.prepare(t, dir)

			staged, err := openStage(t.Context(), dir, testCase.ref)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("openStage() err = %v, want %v", err, testCase.wantErr)
			}

			if hasBase := staged.Base != ""; hasBase != testCase.wantBase {
				t.Fatalf("openStage() base = %q, want a base: %v", staged.Base, testCase.wantBase)
			}
		})
	}
}
