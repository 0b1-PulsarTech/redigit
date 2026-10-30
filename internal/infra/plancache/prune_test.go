package plancache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func seedFile(t *testing.T, path string, age time.Duration) {
	t.Helper()

	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}

	modified := time.Now().Add(-age)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatalf("age %s: %v", path, err)
	}
}

func TestSaveRemovesOnlyStaleTempFiles(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	stale := filepath.Join(store.dir, "old.tmp")
	fresh := filepath.Join(store.dir, "new.tmp")
	other := filepath.Join(store.dir, "notes.txt")

	seedFile(t, stale, 61*time.Minute)
	seedFile(t, fresh, 59*time.Minute)
	seedFile(t, other, 100*time.Hour)

	if err := store.Save(testKey("tidy"), sampleEntry()); err != nil {
		t.Fatalf("Save() err = %v", err)
	}

	testCases := []struct {
		name     string
		path     string
		wantKept bool
	}{
		{"a temp file older than the limit", stale, false},
		{"a temp file inside the limit", fresh, true},
		{"a file that is not the cache's", other, true},
	}

	for _, testCase := range testCases {
		_, err := os.Stat(testCase.path)
		if kept := err == nil; kept != testCase.wantKept {
			t.Errorf("%s: kept = %v, want %v", testCase.name, kept, testCase.wantKept)
		}
	}
}

func TestSaveKeepsTheNewestEntries(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	extra := 5

	for idx := range 32 + extra {
		key := testKey(fmt.Sprintf("entry-%d", idx))
		seedFile(t, filepath.Join(store.dir, key+".gob"), time.Duration(idx+1)*time.Minute)
	}

	if err := store.Save(testKey("newest"), sampleEntry()); err != nil {
		t.Fatalf("Save() err = %v", err)
	}

	left, err := filepath.Glob(filepath.Join(store.dir, "*.gob"))
	if err != nil || len(left) != 32 {
		t.Fatalf("%d entries left (%v), want the newest 32", len(left), err)
	}

	if _, err = store.Load(testKey("newest")); err != nil {
		t.Fatalf("the entry just saved was pruned: %v", err)
	}

	for idx := range 32 + extra {
		path := filepath.Join(store.dir, testKey(fmt.Sprintf("entry-%d", idx))+".gob")
		_, statErr := os.Stat(path)
		wantKept := idx < 31

		if kept := statErr == nil; kept != wantKept {
			t.Errorf("entry %d: kept = %v, want %v", idx, kept, wantKept)
		}
	}
}

func TestLoadRefusesAnOversizedEntry(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	key := testKey("big")
	path := filepath.Join(store.dir, key+".gob")

	seedFile(t, path, 0)

	if err := os.Truncate(path, 64<<20+1); err != nil {
		t.Fatalf("grow the entry: %v", err)
	}

	if _, err := store.Load(key); !errors.Is(err, ErrCorrupt) || !errors.Is(err, errOversized) {
		t.Fatalf("Load() err = %v, want a corrupt, oversized entry", err)
	}
}

func TestSaveLeavesDirectoriesAlone(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	staleTempDir := filepath.Join(store.dir, "dir.tmp")
	oldestEntryDir := filepath.Join(store.dir, "dir.gob")

	for idx := range 32 {
		seedFile(t, filepath.Join(store.dir, fmt.Sprintf("%064x.gob", idx)), time.Minute)
	}

	for _, dir := range []string{staleTempDir, oldestEntryDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}

		aged := time.Now().Add(-100 * time.Hour)
		if err := os.Chtimes(dir, aged, aged); err != nil {
			t.Fatalf("age %s: %v", dir, err)
		}
	}

	if err := store.Save(testKey("dirs"), sampleEntry()); err != nil {
		t.Fatalf("Save() err = %v", err)
	}

	for _, dir := range []string{staleTempDir, oldestEntryDir} {
		if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
			t.Errorf("%s did not survive the prune: %v", filepath.Base(dir), err)
		}
	}
}

func TestLoadKeepsAHotEntryThroughAPrune(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	hot := testKey("hot")

	if err := store.Save(hot, sampleEntry()); err != nil {
		t.Fatalf("Save() err = %v", err)
	}

	old := time.Now().Add(-100 * time.Hour)
	if err := os.Chtimes(filepath.Join(store.dir, hot+".gob"), old, old); err != nil {
		t.Fatalf("age the hot entry: %v", err)
	}

	for idx := range 32 {
		seedFile(t, filepath.Join(store.dir, fmt.Sprintf("%064x.gob", idx)), time.Minute)
	}

	if _, err := store.Load(hot); err != nil {
		t.Fatalf("Load() err = %v", err)
	}

	if err := store.Save(testKey("fresh"), sampleEntry()); err != nil {
		t.Fatalf("Save() err = %v", err)
	}

	if _, err := store.Load(hot); err != nil {
		t.Fatalf("the entry just read was pruned as old: %v", err)
	}
}
