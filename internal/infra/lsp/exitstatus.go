package lsp

import (
	"context"
	"errors"
	"io"
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

func readerEndedCleanly(readErr error) bool {
	// A server may end first once exit is sent; a frame cut short is io.ErrUnexpectedEOF, an error.
	return errors.Is(readErr, context.Canceled) || errors.Is(readErr, io.EOF)
}
