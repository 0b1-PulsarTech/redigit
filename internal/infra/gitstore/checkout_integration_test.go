//go:build integration

package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestHistoryPublishAnswersForWhereTheRefEnded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		phase      string
		action     string
		wantMoved  bool
		wantFailed bool
	}{
		{"git killed after it committed the move", "committed", "kill -KILL $PPID", true, false},
		{"hook refusing before the move", "prepared", "exit 1", false, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			const target = "refs/heads/out"

			dir := newTestRepo(t)
			history := NewHistory(dir, "", Identity{Name: "Proof", Email: "proof@localhost"})

			staged, err := history.Replay(t.Context(), target, replayInputs())
			if err != nil {
				t.Fatalf("Replay() err = %v", err)
			}

			hooks, marker := t.TempDir(), filepath.Join(t.TempDir(), "ran")
			script := fmt.Sprintf(
				"#!/bin/sh\nif [ \"$1\" = %s ] && grep -q ' %s$'; then\n: > %q\n%s\nfi\n",
				testCase.phase, target, marker, testCase.action,
			)

			if err = os.WriteFile(
				filepath.Join(hooks, "reference-transaction"), []byte(script), 0o700,
			); err != nil {
				t.Fatalf("write hook: %v", err)
			}

			runTestGit(t, dir, "config", "--local", "core.hooksPath", hooks)

			err = history.Publish(t.Context(), target, staged)
			if (err != nil) != testCase.wantFailed {
				t.Fatalf("Publish() err = %v, want a failure: %v", err, testCase.wantFailed)
			}

			if _, statErr := os.Stat(marker); statErr != nil {
				t.Fatalf("hook marker: %v, want the hook to have run", statErr)
			}

			if isMoved := refValue(t, dir, target) == staged.Tip; isMoved != testCase.wantMoved {
				t.Fatalf("target moved = %v, want %v", isMoved, testCase.wantMoved)
			}
		})
	}
}

func TestHistoryPublishReadsBackPastACancelledContext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		isAtTip    bool
		wantFailed bool
	}{
		{"ref already on the tip", true, false},
		{"ref elsewhere", false, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			const target = "refs/heads/out"

			dir := newTestRepo(t)
			history := NewHistory(dir, "", Identity{Name: "Proof", Email: "proof@localhost"})

			staged, err := history.Replay(t.Context(), target, replayInputs())
			if err != nil {
				t.Fatalf("Replay() err = %v", err)
			}

			if testCase.isAtTip {
				runTestGit(t, dir, "update-ref", target, staged.Tip)
			}

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err = history.Publish(ctx, target, staged)
			if (err != nil) != testCase.wantFailed {
				t.Fatalf("Publish() err = %v, want a failure: %v", err, testCase.wantFailed)
			}
		})
	}
}

func TestHistoryPublishRefusesAMovedRef(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		isSeed bool
	}{
		{"absent at start, created meanwhile", false},
		{"present at start, moved meanwhile", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			const target = "refs/heads/out"

			dir := newTestRepo(t)
			ctx := t.Context()
			history := NewHistory(dir, "", Identity{Name: "Proof", Email: "proof@localhost"})
			commitTestFiles(t, dir, "seed", map[string][]byte{"a.go": []byte("One\n")})

			if testCase.isSeed {
				runTestGit(t, dir, "update-ref", target, "HEAD")
			}

			staged, err := history.Replay(ctx, target, replayInputs())
			if err != nil {
				t.Fatalf("Replay() err = %v", err)
			}

			commitTestFiles(t, dir, "moved", map[string][]byte{"a.go": []byte("Two\n")})
			runTestGit(t, dir, "update-ref", target, "HEAD")

			moved := refValue(t, dir, target)

			err = history.Publish(ctx, target, staged)
			if !errors.Is(err, ErrRefMoved) || !strings.Contains(err.Error(), "cannot lock ref") {
				t.Fatalf("Publish() err = %v, want ErrRefMoved carrying git's reason", err)
			}

			if got := refValue(t, dir, target); got != moved {
				t.Fatalf("target = %q, want it left at %q", got, moved)
			}
		})
	}
}

func TestHistoryDiscardCarriesGitsReason(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	history := NewHistory(dir, "", Identity{Name: "Proof", Email: "proof@localhost"})

	staged, err := history.Replay(t.Context(), "refs/heads/out", replayInputs())
	if err != nil {
		t.Fatalf("Replay() err = %v", err)
	}

	lock := filepath.Join(dir, ".git", filepath.FromSlash(staged.Scratch)+".lock")
	if err = os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	err = history.Discard(t.Context(), staged)
	if err == nil || !strings.Contains(err.Error(), "File exists") || errors.Is(err, ErrRefMoved) {
		t.Fatalf("Discard() err = %v, want git's lock complaint and no ErrRefMoved", err)
	}
}
