package golang

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// readSource reads a repo-relative path against the snapshot's checked-out
// tree. The rule ports carry no Tree dependency of their own: Snapshot.Root
// is documented as the final tree on disk, so a Go-specific rule reads the
// file directly rather than caching it through an unrequested seam.
func readSource(snap entities.Snapshot, relPath string) ([]byte, error) {
	// gosec flags a variable-built path as traversal-risky; here the variable
	// is the repo-relative path the same indexing run's Snapshot.Files
	// already enumerated, not untrusted external input.
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
