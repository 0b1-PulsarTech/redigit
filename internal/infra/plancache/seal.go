package plancache

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
)

//go:generate go tool -modfile=../../../tools/go.mod mockgen -source=seal.go -destination=seal_mock_test.go -package=plancache

type sealedFile interface {
	io.WriteCloser
	Sync() error
}

func writeSealed(out sealedFile, payload []byte) error {
	// The digest rides last, so a flipped byte anywhere before it is a corrupt entry.
	sum := sha256.Sum256(payload)

	if _, err := out.Write(append(payload, sum[:]...)); err != nil {
		return errors.Join(fmt.Errorf("write the cache entry: %w", err), out.Close())
	}

	// Synced before the rename, so a crash never publishes an entry that was never written.
	if err := out.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync the cache entry: %w", err), out.Close())
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close the cache temp file: %w", err)
	}

	return nil
}

func encodeEntry(entry Entry) ([]byte, error) {
	var buffer bytes.Buffer

	encoder := gob.NewEncoder(&buffer)

	if err := encoder.Encode(header{Magic: fileMagic, Version: formatVersion}); err != nil {
		return nil, fmt.Errorf("encode the cache header: %w", err)
	}

	if err := encoder.Encode(entry); err != nil {
		return nil, fmt.Errorf("encode the cache entry: %w", err)
	}

	return buffer.Bytes(), nil
}
