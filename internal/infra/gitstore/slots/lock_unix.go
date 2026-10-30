//go:build unix

package slots

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func tryLock(path string) (*os.File, bool, error) {
	const lockMode = 0o600

	// #nosec G304 -- the lock path is built by this adapter under its own slots dir.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, lockMode)
	if err != nil {
		return nil, false, fmt.Errorf("open slot lock: %w", err)
	}

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return file, true, nil
	}

	closeErr := file.Close()
	if errors.Is(err, syscall.EWOULDBLOCK) {
		if closeErr != nil {
			return nil, false, fmt.Errorf("close slot lock: %w", closeErr)
		}

		return nil, false, nil
	}

	return nil, false, errors.Join(fmt.Errorf("lock slot: %w", err), closeErr)
}
