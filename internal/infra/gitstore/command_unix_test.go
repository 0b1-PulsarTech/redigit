//go:build unix

package gitstore

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestRunGitDetachedLeavesTheCallersProcessGroup(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("ps is not installed")
	}

	testCases := []struct {
		name       string
		run        func(t *testing.T, dir string) ([]byte, error)
		isSameAsUs bool
	}{
		{
			"plain run shares the group",
			func(t *testing.T, dir string) ([]byte, error) {
				t.Helper()

				return runGit(t.Context(), dir, "-c", "alias.pg=!ps -o pgid= -p $$", "pg")
			},
			true,
		},
		{
			"detached run leads its own group",
			func(t *testing.T, dir string) ([]byte, error) {
				t.Helper()

				return runGitDetached(t.Context(), dir, "-c", "alias.pg=!ps -o pgid= -p $$", "pg")
			},
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out, err := testCase.run(t, t.TempDir())
			if err != nil {
				t.Fatalf("run git alias: %v", err)
			}

			group, err := strconv.Atoi(strings.TrimSpace(string(out)))
			if err != nil {
				t.Fatalf("parse process group %q: %v", out, err)
			}

			if isSame := group == syscall.Getpgrp(); isSame != testCase.isSameAsUs {
				t.Fatalf("git ran in group %d, ours is %d", group, syscall.Getpgrp())
			}
		})
	}
}
