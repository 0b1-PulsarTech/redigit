package plancache

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadAgreeOnTheCap(t *testing.T) {
	t.Parallel()

	payload, err := encodeEntry(sampleEntry())
	if err != nil {
		t.Fatalf("encodeEntry() err = %v", err)
	}

	size := int64(len(payload) + sha256.Size)

	testCases := []struct {
		name        string
		saveCap     int64
		loadCap     int64
		wantSaveErr error
		wantLoadErr error
	}{
		{"at the cap both ways", size, size, nil, nil},
		{"one byte over on save", size - 1, size, ErrTooLarge, nil},
		{"one byte over on load", size, size - 1, nil, errOversized},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			key := testKey("cap")

			err := Open(dir).Capped(testCase.saveCap).Save(key, sampleEntry())
			if !errors.Is(err, testCase.wantSaveErr) {
				t.Fatalf("Save() err = %v, want %v", err, testCase.wantSaveErr)
			}

			if testCase.wantSaveErr != nil {
				if listed, _ := os.ReadDir(dir); len(listed) != 0 {
					t.Fatalf("Save() left %d files behind a refusal", len(listed))
				}

				if err = Open(dir).Save(key, sampleEntry()); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}

			_, err = Open(dir).Capped(testCase.loadCap).Load(key)
			if !errors.Is(err, testCase.wantLoadErr) {
				t.Fatalf("Load() err = %v, want %v", err, testCase.wantLoadErr)
			}

			if testCase.wantLoadErr != nil && !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Load() err = %v, want it to be a corrupt entry too", err)
			}
		})
	}
}

func TestLoadStopsReadingAtTheCap(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir()).Capped(1 << 10)
	key := testKey("grown")
	path := filepath.Join(store.dir, key+".gob")

	seedFile(t, path, 0)

	if err := os.Truncate(path, 1<<30); err != nil {
		t.Fatalf("grow the entry: %v", err)
	}

	if _, err := store.Load(key); !errors.Is(err, errOversized) {
		t.Fatalf("Load() err = %v, want an oversized entry", err)
	}
}
