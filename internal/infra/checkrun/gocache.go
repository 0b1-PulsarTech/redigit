package checkrun

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const goCacheDir = "gocache"

func withGoCache(root string, run, keyed []string) ([]string, string, error) {
	const (
		keyLen  = 32
		dirMode = 0o750
	)

	// Go's build cache is blind to some cgo inputs (CPATH), so each hashed env gets its own.
	digest := sha256.Sum256([]byte(strings.Join(keyed, "\x00")))
	dir := filepath.Join(root, goCacheDir, hex.EncodeToString(digest[:])[:keyLen])

	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, "", fmt.Errorf("create go build cache: %w", err)
	}

	// The mtime is the last-use time that Prune ages the directory by.
	now := time.Now()
	if err := os.Chtimes(dir, now, now); err != nil {
		return nil, "", fmt.Errorf("touch go build cache: %w", err)
	}

	env := make([]string, 0, len(run)+1)

	for _, entry := range run {
		if !strings.HasPrefix(entry, "GOCACHE=") {
			env = append(env, entry)
		}
	}

	return append(env, "GOCACHE="+dir), dir, nil
}

func (runner *Runner) Prune(now time.Time) error {
	const maxAge = 14 * 24 * time.Hour

	root := filepath.Join(runner.cfg.CacheDir, goCacheDir)

	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("list go build caches: %w", err)
	}

	runner.mutex.Lock()
	defer runner.mutex.Unlock()

	var failures []error

	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		if _, isUsed := runner.used[dir]; isUsed || !entry.IsDir() {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			failures = append(failures, infoErr)

			continue
		}

		if now.Sub(info.ModTime()) > maxAge {
			failures = append(failures, os.RemoveAll(dir))
		}
	}

	return errors.Join(failures...)
}
