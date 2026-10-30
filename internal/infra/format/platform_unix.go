//go:build unix

package format

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

const (
	readFlags = syscall.O_RDONLY | syscall.O_NONBLOCK

	argBudget = 30000
)

func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// the Process signal keeps its done-guard, so a reaped pid never receives the group kill
		if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("signal process: %w", err)
		}

		// a negative pid signals the whole group the tool leads; a setsid escape is out of scope
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return fmt.Errorf("kill process group: %w", err)
		}

		return nil
	}
}

func runGroup(cmd *exec.Cmd) error {
	err := cmd.Run()

	// Sweeps a background child of a tool that exited 0; accepted risk: the group is empty and its
	// pid must wrap and be reused as a group leader within microseconds
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	if err != nil {
		return fmt.Errorf("run tool: %w", err)
	}

	return nil
}

func argTooLong(err error) bool {
	return errors.Is(err, syscall.E2BIG)
}
