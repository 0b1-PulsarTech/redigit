package format

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
)

func readRows(path string) ([]manifest.Row, error) {
	file, err := os.Open(path) // #nosec G304 -- caller-supplied manifest path.
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}

	defer func() { _ = file.Close() }()

	return manifest.Decode(file) //nolint:wrapcheck // Decode names the failing line
}

func publish(opts options, rows []manifest.Row, fresh map[string][]byte) error {
	const (
		dirMode  = 0o750
		fileMode = 0o600
	)

	if err := os.MkdirAll(opts.outBlobDir, dirMode); err != nil {
		return fmt.Errorf("make out blob dir: %w", err)
	}

	done := map[string]struct{}{}

	for _, row := range rows {
		for _, blob := range row.Blobs {
			if _, copied := done[blob.Handle]; copied || blob.Handle == "" {
				continue
			}

			done[blob.Handle] = struct{}{}

			content, found := fresh[blob.Handle]
			if !found {
				var err error
				if content, err = readBlob(opts.blobDir, blob.Handle); err != nil {
					return err
				}
			}

			target := filepath.Join(opts.outBlobDir, blob.Handle)
			if err := os.WriteFile(target, content, fileMode); err != nil {
				return fmt.Errorf("write blob: %w", err)
			}
		}
	}

	return writeManifest(opts.out, rows)
}

func writeManifest(path string, rows []manifest.Row) error {
	file, err := os.Create(path) // #nosec G304 -- caller-supplied output path.
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}

	if err = manifest.Encode(file, rows); err != nil {
		_ = file.Close()

		return err //nolint:wrapcheck // Encode names the failing step
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("close manifest: %w", err)
	}

	return nil
}
