package plancache

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var errOversized = errors.New("larger than any entry this tool writes")

func (store Store) Load(key string) (Entry, error) {
	path, err := store.path(key)
	if err != nil {
		return Entry{}, err
	}

	// #nosec G304 -- path is the cache dir plus a validated hex digest.
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Entry{}, ErrMiss
	}

	if err != nil {
		return Entry{}, fmt.Errorf("%w: open: %w", ErrCorrupt, err)
	}

	defer func() { _ = file.Close() }()

	// One byte past the cap tells an entry at the cap from one over it.
	raw, err := io.ReadAll(io.LimitReader(file, store.maxSize()+1))
	if err != nil {
		return Entry{}, fmt.Errorf("%w: read: %w", ErrCorrupt, err)
	}

	if int64(len(raw)) > store.maxSize() {
		return Entry{}, fmt.Errorf("%w: %w", ErrCorrupt, errOversized)
	}

	if len(raw) < sha256.Size {
		return Entry{}, fmt.Errorf("%w: too short to hold its digest", ErrCorrupt)
	}

	payload, sealed := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	if sum := sha256.Sum256(payload); !bytes.Equal(sum[:], sealed) {
		return Entry{}, fmt.Errorf("%w: the digest does not match", ErrCorrupt)
	}

	entry, err := decodeEntry(bytes.NewReader(payload))
	if err == nil {
		// A hit is use: refresh the mtime so prune keeps what is read, not just what was written.
		// Ignored on purpose: a hit must not fail because the mtime refresh failed.
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}

	return entry, err
}

func decodeEntry(in io.Reader) (Entry, error) {
	decoder := gob.NewDecoder(bufio.NewReader(in))

	var head header
	if err := decoder.Decode(&head); err != nil || head.Magic != fileMagic {
		return Entry{}, fmt.Errorf("%w: header", ErrCorrupt)
	}

	if head.Version != formatVersion {
		return Entry{}, ErrMiss
	}

	var entry Entry
	if err := decoder.Decode(&entry); err != nil {
		return Entry{}, fmt.Errorf("%w: body: %w", ErrCorrupt, err)
	}

	// Gob drops an empty map, but a settled ordering is never nil.
	if entry.Ordering.At == nil {
		entry.Ordering.At = map[entities.DeclKey]entities.Position{}
	}

	return entry, nil
}
