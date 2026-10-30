//go:build !unix

package format

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const (
	readFlags = os.O_RDONLY

	// A .cmd shim runs through cmd.exe, which stops at 8191 characters, not CreateProcess's 32767,
	// and that failure is a generic invalid parameter, so it is budgeted, not matched. 7500 leaves
	// room for the /d /s /c prefix and the escaping.
	argBudget = 7500
)

func isolate(*exec.Cmd) {
	// no process groups here: the default Cancel kills only the tool itself
}

func runGroup(cmd *exec.Cmd) error {
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run tool: %w", err)
	}

	return nil
}

func argTooLong(err error) bool {
	// CreateProcess returns ERROR_FILENAME_EXCED_RANGE (206) for a command line past 32767 units
	const errFilenameExcedRange = syscall.Errno(206)

	return errors.Is(err, errFilenameExcedRange)
}
