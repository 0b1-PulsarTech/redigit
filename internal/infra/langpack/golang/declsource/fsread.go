package declsource

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func ReadSource(snap entities.Snapshot, relPath string) ([]byte, error) {
	// gosec flags a variable-built path as traversal-risky; here the variable
	// is the repo-relative path the same indexing run's Snapshot.Files
	// already enumerated, not untrusted external input.
	source, err := os.ReadFile(filepath.Join(snap.Root, relPath)) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relPath, err)
	}

	return source, nil
}

func SliceSpan(source []byte, span entities.Span) string {
	if span.Start < 0 || span.End > len(source) || span.Start > span.End {
		return ""
	}

	return string(source[span.Start:span.End])
}
