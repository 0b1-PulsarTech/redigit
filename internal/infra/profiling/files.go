package profiling

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

func createFile(path string) (*os.File, error) {
	const fileMode = 0o600

	// #nosec G304 -- the operator names their own profile output path.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return nil, fmt.Errorf("create profile file: %w", err)
	}

	return file, nil
}

func closeFile(file *os.File) error {
	if err := file.Close(); err != nil {
		return fmt.Errorf("close profile file: %w", err)
	}

	return nil
}

func distinct(opts Options) error {
	// One file written by two sinks would interleave them into neither format.
	var paths []string

	for _, path := range []string{opts.CPUProfile, opts.MemProfile, opts.Trace} {
		if path == "" {
			continue
		}

		path = filepath.Clean(path)
		if slices.Contains(paths, path) {
			return ErrSamePath
		}

		paths = append(paths, path)
	}

	return nil
}
