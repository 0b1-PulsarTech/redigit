//go:build unix

package batch

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestIsolateGivesTheProcessItsOwnGroup(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		isIsolated bool
	}{
		{"isolated process leads its own group", true},
		{"plain process stays in ours", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.CommandContext(t.Context(), "sleep", "60")
			if testCase.isIsolated {
				Isolate(cmd)
			}

			if err := cmd.Start(); err != nil {
				t.Fatalf("start: %v", err)
			}

			t.Cleanup(func() { _ = cmd.Wait() })

			group, err := syscall.Getpgid(cmd.Process.Pid)
			if err != nil {
				t.Fatalf("read process group: %v", err)
			}

			if isOwn := group == cmd.Process.Pid; isOwn != testCase.isIsolated {
				t.Fatalf("process %d is in group %d", cmd.Process.Pid, group)
			}
		})
	}
}
