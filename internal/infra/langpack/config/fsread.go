package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// readSource reads a repo-relative path against the snapshot's checked-out
// tree, the same way the Go pack does: Snapshot.Root is documented as the
// final tree on disk, so a rule reads the file directly rather than carrying
// a Tree seam nobody asked for.
func readSource(snap entities.Snapshot, relPath string) ([]byte, error) {
	// gosec flags a variable-built path as traversal-risky; the variable is a
	// repo-relative path this run's Snapshot.Files already enumerated.
	source, err := os.ReadFile(filepath.Join(snap.Root, relPath)) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relPath, err)
	}

	return source, nil
}

// sliceSpan returns the text a Decl's Span covers, bounds-checked against a
// source that may not be the exact bytes the Span was computed from.
func sliceSpan(source []byte, span entities.Span) string {
	if span.Start < 0 || span.End > len(source) || span.Start > span.End {
		return ""
	}

	return string(source[span.Start:span.End])
}
