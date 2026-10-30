package lsp

import (
	"errors"
	"os/exec"
)

func cleanExit(waitErr error) error {
	// After a completed shutdown/exit handshake the server decides how it ends: gopls v0.23.0
	// dies of SIGPIPE on its closed stdin, so any exit status, a signal included, is a clean close.
	if _, exited := errors.AsType[*exec.ExitError](waitErr); exited {
		return nil
	}

	return waitErr
}
