package format

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func refuseOverwrite(opts options) error {
	names := []string{opts.out, opts.manifest, opts.configPath, opts.blobDir, opts.outBlobDir}
	places := make(map[string]string, len(names))

	for _, name := range names {
		place, err := resolve(name)
		if err != nil {
			return fmt.Errorf("%w: resolve path: %w", write.ErrUsage, err)
		}

		places[name] = place
	}

	out, blobs, outBlobs := places[opts.out], places[opts.blobDir], places[opts.outBlobDir]

	switch {
	case sameFile(opts.out, out, opts.manifest, places[opts.manifest]),
		sameFile(opts.out, out, opts.configPath, places[opts.configPath]),
		within(blobs, out), within(outBlobs, out):
		return fmt.Errorf("%w: --out must not be its input", write.ErrUsage)
	case within(blobs, outBlobs), within(outBlobs, blobs):
		return fmt.Errorf("%w: --out-blob-dir must not be its input", write.ErrUsage)
	}

	return nil
}

func resolve(name string) (string, error) {
	current, err := filepath.Abs(name)
	if err != nil {
		return "", fmt.Errorf("absolute path: %w", err)
	}

	var tail string

	for {
		found, evalErr := filepath.EvalSymlinks(current)
		if evalErr == nil {
			return filepath.Join(found, tail), nil
		}

		parent := filepath.Dir(current)
		if !errors.Is(evalErr, fs.ErrNotExist) || parent == current {
			return "", fmt.Errorf("resolve symlinks: %w", evalErr)
		}

		tail, current = filepath.Join(filepath.Base(current), tail), parent
	}
}

func sameFile(left, leftPlace, right, rightPlace string) bool {
	if leftPlace == rightPlace {
		return true
	}

	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)

	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

func within(dir, name string) bool {
	rel, err := filepath.Rel(dir, name)

	return err == nil && filepath.IsLocal(rel)
}
