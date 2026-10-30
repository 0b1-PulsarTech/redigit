package verifycache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func goodKey(fill string) string { return strings.Repeat(fill, 64) }

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		outcome verifying.Outcome
	}{
		{"a pass", verifying.Outcome{Step: "go vet"}},
		{
			"a failure with a tail",
			verifying.Outcome{IsFailed: true, Step: "go build", Tail: "x.go:1: bad\n"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			store := New(filepath.Join(t.TempDir(), "nested", "verify"))

			if _, isHit, err := store.Get(context.Background(), goodKey("a")); isHit || err != nil {
				t.Fatalf("empty store: hit = %v err = %v, want a clean miss", isHit, err)
			}

			if err := store.Put(context.Background(), goodKey("a"), testCase.outcome); err != nil {
				t.Fatalf("put: %v", err)
			}

			got, isHit, err := store.Get(context.Background(), goodKey("a"))
			if err != nil || !isHit || got != testCase.outcome {
				t.Fatalf("get = %+v hit = %v err = %v, want %+v", got, isHit, err, testCase.outcome)
			}

			if _, isHit, _ = store.Get(context.Background(), goodKey("b")); isHit {
				t.Fatal("another key must miss")
			}
		})
	}
}

func TestStoreCorruptEntryIsAMiss(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
	}{
		{"not json", "{{{"},
		{"empty file", ""},
		{"truncated", `{"Step":"go b`},
		{"wrong shape", `[1,2]`},
		{"empty object", `{}`},
		{"null", `null`},
		{"a foreign object", `{"x":1}`},
		{"no version", `{"failed":false,"step":"go vet","tail":""}`},
		{"another version", `{"version":2,"failed":false,"step":"go vet","tail":""}`},
		{"an unknown member", `{"version":1,"failed":false,"step":"go vet","tail":"","x":1}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			store := New(t.TempDir())
			if err := store.Put(
				context.Background(),
				goodKey("c"),
				verifying.Outcome{},
			); err != nil {
				t.Fatalf("put: %v", err)
			}

			path := store.path(goodKey("c"))
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("corrupt: %v", err)
			}

			_, isHit, err := store.Get(context.Background(), goodKey("c"))
			if isHit || err != nil {
				t.Fatalf("hit = %v err = %v, want a miss without an error", isHit, err)
			}
		})
	}
}

func TestStoreRejectsBadKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"short", "abc"},
		{"traversal", "../" + strings.Repeat("a", 61)},
		{"non hex", strings.Repeat("z", 64)},
		{"upper case", strings.Repeat("A", 64)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			store := New(t.TempDir())

			if _, _, err := store.Get(
				context.Background(),
				testCase.key,
			); !errors.Is(
				err,
				ErrBadKey,
			) {
				t.Fatalf("get err = %v, want ErrBadKey", err)
			}

			if err := store.Put(
				context.Background(),
				testCase.key,
				verifying.Outcome{},
			); !errors.Is(
				err,
				ErrBadKey,
			) {
				t.Fatalf("put err = %v, want ErrBadKey", err)
			}
		})
	}
}

func TestStorePutFailsWhenTheDirIsAFile(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("blocker: %v", err)
	}

	if err := New(
		blocker,
	).Put(context.Background(), goodKey("d"), verifying.Outcome{}); err == nil {
		t.Fatal("put under a regular file must fail")
	}
}

func TestStoreConcurrentWriters(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	start := make(chan struct{})
	outcome := verifying.Outcome{Step: "go vet", Tail: strings.Repeat("tail ", 1000)}

	var group sync.WaitGroup

	errs := make(chan error, 16)

	for range 16 {
		group.Go(func() {
			<-start

			errs <- store.Put(context.Background(), goodKey("e"), outcome)
		})
	}

	close(start)
	group.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent put: %v", err)
		}
	}

	got, isHit, err := store.Get(context.Background(), goodKey("e"))
	if err != nil || !isHit || got != outcome {
		t.Fatalf("after the race: hit = %v err = %v, want the whole outcome", isHit, err)
	}

	entries, readErr := os.ReadDir(filepath.Dir(store.path(goodKey("e"))))
	if readErr != nil || len(entries) != 1 {
		t.Fatalf(
			"entries = %d err = %v, want one file and no temp leftovers",
			len(entries),
			readErr,
		)
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/redigit-cache-probe")

	got, err := DefaultDir()
	if err != nil || got != "/tmp/redigit-cache-probe/redigit/verify" {
		t.Fatalf("dir = %q err = %v", got, err)
	}
}
