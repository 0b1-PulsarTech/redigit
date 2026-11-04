package lint

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type measuredRows struct {
	rows     []linting.Row
	measured bool
}

func readRows(manifestPath, blobDir string) (measuredRows, error) {
	file, err := os.Open(manifestPath) // #nosec G304 -- caller-supplied manifest path.
	if err != nil {
		return measuredRows{}, fmt.Errorf("open manifest: %w", err)
	}

	defer func() { _ = file.Close() }()

	decoded, err := manifest.Decode(file)
	if err != nil {
		return measuredRows{}, err //nolint:wrapcheck // Decode names the failing row
	}

	if len(decoded) == 0 {
		return measuredRows{}, fmt.Errorf("%w: the manifest holds no row", rewriting.ErrManifest)
	}

	slices.SortStableFunc(decoded, func(left, right manifest.Row) int {
		return cmp.Compare(left.Position, right.Position)
	})

	if blobDir == "" {
		rows := make([]linting.Row, len(decoded))
		for index, row := range decoded {
			rows[index] = linting.Row{Position: row.Position, Subject: row.Subject}
		}

		return measuredRows{rows: rows}, nil
	}

	return addedLines(decoded, blobDir)
}

func addedLines(decoded []manifest.Row, blobDir string) (measuredRows, error) {
	// measured is set only when a blob is read: rows that hold none measured nothing.
	last := map[string]int{}
	result := measuredRows{rows: make([]linting.Row, 0, len(decoded))}

	for _, row := range decoded {
		out := linting.Row{Position: row.Position, Subject: row.Subject}

		for _, blob := range row.Blobs {
			if blob.Mode == "160000" {
				continue
			}

			content, err := readBlob(blobDir, blob.Handle)
			if err != nil {
				return measuredRows{}, fmt.Errorf(
					"row %d blob %q: %w", row.Position, blob.Path, err,
				)
			}

			result.measured = true
			lines := lineCount(content)
			out.Files = append(out.Files, linting.File{
				Path:  blob.Path,
				Added: lines - last[blob.Path],
				Kind:  linting.Classify(blob.Path, content),
			})
			last[blob.Path] = lines
		}

		result.rows = append(result.rows, out)
	}

	return result, nil
}

func readBlob(blobDir, handle string) ([]byte, error) {
	if !filepath.IsLocal(handle) || strings.ContainsRune(handle, filepath.Separator) {
		return nil, fmt.Errorf("%w: handle %q is not a plain file name", write.ErrUsage, handle)
	}

	content, err := os.ReadFile(
		filepath.Join(blobDir, handle),
	) // #nosec G304 -- handle checked above.
	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}

	return content, nil
}

func lineCount(content []byte) int {
	if linting.IsBinary(content) {
		return 0
	}

	lines := bytes.Count(content, []byte{'\n'})
	if len(content) > 0 && content[len(content)-1] != '\n' {
		lines++
	}

	return lines
}
