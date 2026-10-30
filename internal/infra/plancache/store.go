package plancache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	fileMagic = "redigit-plancache"
)

var (
	ErrMiss     = errors.New("plan cache miss")
	ErrCorrupt  = errors.New("plan cache entry is corrupt")
	ErrBadKey   = errors.New("plan cache key is not a sha256 digest")
	ErrTooLarge = errors.New("plan cache entry is larger than the cache keeps")
)

type header struct {
	Magic   string
	Version int
}

type Store struct {
	dir   string
	limit int64
}

func Open(dir string) Store { return Store{dir: dir} }

func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find the user cache dir: %w", err)
	}

	return filepath.Join(base, "redigit", "plancache"), nil
}

func (store Store) Save(key string, entry Entry) (err error) {
	const dirMode = 0o700

	// A temp file plus a rename: a reader sees the whole entry or none of it.
	var final string

	if final, err = store.path(key); err != nil {
		return err
	}

	var payload []byte

	if payload, err = encodeEntry(entry); err != nil {
		return err
	}

	// Load refuses these, so saving one would only be redone on every run.
	if int64(len(payload)+sha256.Size) > store.maxSize() {
		return ErrTooLarge
	}

	if err = os.MkdirAll(store.dir, dirMode); err != nil {
		return fmt.Errorf("create the cache dir: %w", err)
	}

	// A dir left looser by an older run or another tool would let others plant entries.
	if err = os.Chmod(store.dir, dirMode); err != nil {
		return fmt.Errorf("tighten the cache dir: %w", err)
	}

	var staging *os.File

	if staging, err = os.CreateTemp(store.dir, "*.tmp"); err != nil {
		return fmt.Errorf("create a cache temp file: %w", err)
	}

	defer func() {
		if err != nil {
			_ = os.Remove(staging.Name())
		}
	}()

	if err = writeSealed(staging, payload); err != nil {
		return err
	}

	if err = os.Rename(staging.Name(), final); err != nil {
		return fmt.Errorf("publish the cache entry: %w", err)
	}

	store.prune()

	return nil
}

func (store Store) Capped(maxBytes int64) Store {
	store.limit = maxBytes

	return store
}

func (store Store) maxSize() int64 {
	if store.limit > 0 {
		return store.limit
	}

	const maxEntrySize = 64 << 20

	return maxEntrySize
}

func (store Store) path(key string) (string, error) {
	const keyBytes = 32

	if raw, err := hex.DecodeString(key); err != nil || len(raw) != keyBytes {
		return "", ErrBadKey
	}

	return filepath.Join(store.dir, key+".gob"), nil
}
