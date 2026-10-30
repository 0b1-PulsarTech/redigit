package plancache

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type aged struct {
	path     string
	modified time.Time
}

func (store Store) prune() {
	const (
		staleTempAge = time.Hour
		keptEntries  = 32
	)

	// Best effort: a cache that cannot tidy itself still answers.
	listed, err := os.ReadDir(store.dir)
	if err != nil {
		return
	}

	var entries []aged

	for _, item := range listed {
		info, infoErr := item.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}

		found := aged{path: filepath.Join(store.dir, item.Name()), modified: info.ModTime()}

		switch {
		case strings.HasSuffix(item.Name(), ".tmp"):
			if time.Since(found.modified) > staleTempAge {
				_ = os.Remove(found.path)
			}
		case strings.HasSuffix(item.Name(), ".gob"):
			entries = append(entries, found)
		}
	}

	slices.SortFunc(
		entries,
		func(left, right aged) int { return right.modified.Compare(left.modified) },
	)

	for _, old := range entries[min(keptEntries, len(entries)):] {
		_ = os.Remove(old.path)
	}
}
