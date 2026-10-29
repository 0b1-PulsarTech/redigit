package rules

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"
)

const maxConfigBytes = 1 << 20

var (
	ErrNotRegular = errors.New("not a regular file")
	ErrTooLarge   = errors.New("larger than the 1 MiB limit")
)

func ReadBounded(path string) ([]byte, error) {
	// O_NONBLOCK stops a FIFO open waiting for a writer; Fstat then refuses it.
	const flags = os.O_RDONLY | syscall.O_NONBLOCK

	file, err := os.OpenFile(path, flags, 0) // #nosec G304 -- caller-supplied config path.
	if err != nil {
		return nil, bare(err)
	}

	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return nil, bare(err)
	}

	if !stat.Mode().IsRegular() {
		return nil, ErrNotRegular
	}

	// One byte past the cap tells a file that grew after the stat from one that fits.
	content, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return nil, bare(err)
	}

	if len(content) > maxConfigBytes {
		return nil, ErrTooLarge
	}

	return content, nil
}

func bare(err error) error {
	// The caller names the path once, so the os error loses its own copy.
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}

	return err
}
