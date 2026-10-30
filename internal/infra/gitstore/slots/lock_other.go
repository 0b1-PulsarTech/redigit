//go:build !unix

package slots

import "os"

func tryLock(string) (*os.File, bool, error) {
	// A lockfile created with O_EXCL would outlive a crash and block every later run.
	return nil, false, ErrSlotsUnsupported
}
