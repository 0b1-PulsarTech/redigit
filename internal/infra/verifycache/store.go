package verifycache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

const (
	recordVersion = 1
)

var (
	ErrBadKey = errors.New("bad cache key")

	_ verifying.Cache = (*Store)(nil)
)

func isKey(key string) bool {
	const hexDigestLen = 64

	return len(key) == hexDigestLen && !strings.ContainsFunc(key, func(char rune) bool {
		return (char < '0' || char > '9') && (char < 'a' || char > 'f')
	})
}

type (
	Store struct {
		dir string
	}

	record struct {
		Version int    `json:"version"`
		Failed  bool   `json:"failed"`
		Step    string `json:"step"`
		Tail    string `json:"tail"`
	}
)

func New(dir string) *Store { return &Store{dir: dir} }

func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache dir: %w", err)
	}

	return filepath.Join(base, "redigit", "verify"), nil
}

func (store *Store) Get(ctx context.Context, key string) (verifying.Outcome, bool, error) {
	if !isKey(key) {
		return verifying.Outcome{}, false, ErrBadKey
	}

	if err := ctx.Err(); err != nil {
		return verifying.Outcome{}, false, fmt.Errorf("read cache entry: %w", err)
	}

	content, err := os.ReadFile(store.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return verifying.Outcome{}, false, nil
	}

	if err != nil {
		return verifying.Outcome{}, false, fmt.Errorf("read cache entry: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()

	var stored record
	if decoder.Decode(&stored) != nil || stored.Version != recordVersion {
		// A torn, foreign or other-version file is only a miss: the check reruns and rewrites it.
		return verifying.Outcome{}, false, nil
	}

	return verifying.Outcome{
		IsFailed: stored.Failed,
		Step:     stored.Step,
		Tail:     stored.Tail,
	}, true, nil
}

func (store *Store) Put(ctx context.Context, key string, outcome verifying.Outcome) error {
	const dirMode = 0o750

	if !isKey(key) {
		return ErrBadKey
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write cache entry: %w", err)
	}

	content, err := json.Marshal(
		record{
			Version: recordVersion,
			Failed:  outcome.IsFailed,
			Step:    outcome.Step,
			Tail:    outcome.Tail,
		},
	)
	if err != nil {
		return fmt.Errorf("encode cache entry: %w", err)
	}

	final := store.path(key)
	if err = os.MkdirAll(filepath.Dir(final), dirMode); err != nil {
		return fmt.Errorf("make cache dir: %w", err)
	}

	return writeAtomic(final, content)
}

func (store *Store) path(key string) string {
	return filepath.Join(store.dir, key[:2], key+".json")
}

func writeAtomic(final string, content []byte) error {
	staged, err := os.CreateTemp(filepath.Dir(final), ".entry-*")
	if err != nil {
		return fmt.Errorf("create cache temp: %w", err)
	}

	_, writeErr := staged.Write(content)
	closeErr := staged.Close()

	if err = errors.Join(writeErr, closeErr); err == nil {
		// Rename is atomic, so a reader sees the old entry or the whole new one.
		err = os.Rename(staged.Name(), final)
	}

	if err != nil {
		_ = os.Remove(staged.Name())

		return fmt.Errorf("write cache entry: %w", err)
	}

	return nil
}
