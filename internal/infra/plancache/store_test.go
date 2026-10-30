package plancache

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

func testKey(seed string) string {
	key, _ := Inputs{Commit: seed}.Key()

	return key
}

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	key := testKey("round-trip")
	want := sampleEntry()

	if err := store.Save(key, want); err != nil {
		t.Fatalf("Save() err = %v", err)
	}

	got, err := store.Load(key)
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}

	if entries, _ := os.ReadDir(store.dir); len(entries) != 1 {
		t.Fatalf("cache dir holds %d files, want only the entry", len(entries))
	}
}

func TestStoreOverwrites(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	key := testKey("overwrite")

	for _, note := range []string{"first", "second"} {
		entry := sampleEntry()
		entry.OrderNote = note

		if err := store.Save(key, entry); err != nil {
			t.Fatalf("Save(%s) err = %v", note, err)
		}
	}

	got, err := store.Load(key)
	if err != nil || got.OrderNote != "second" {
		t.Fatalf("Load() = %q, %v, want the second save", got.OrderNote, err)
	}
}

func TestStoreRestoresAnEmptyOrdering(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		at   map[entities.DeclKey]entities.Position
	}{
		{"an empty map", map[entities.DeclKey]entities.Position{}},
		{"no map at all", nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			store := Open(t.TempDir())
			key := testKey("empty")
			entry := Pack(indexing.Result{Graph: entities.NewGraph(nil, nil, nil)})
			entry.Ordering = entities.Ordering{At: testCase.at, Rounds: 1}

			if err := store.Save(key, entry); err != nil {
				t.Fatalf("Save() err = %v", err)
			}

			got, err := store.Load(key)
			if err != nil || got.Ordering.At == nil {
				t.Fatalf("Load() At = %v, %v, want a non-nil map", got.Ordering.At, err)
			}
		})
	}
}

func encoded(t *testing.T, values ...any) []byte {
	t.Helper()

	var buffer bytes.Buffer

	encoder := gob.NewEncoder(&buffer)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			t.Fatalf("encode %T: %v", value, err)
		}
	}

	return buffer.Bytes()
}

func TestStoreLoadFailures(t *testing.T) {
	t.Parallel()

	good := encoded(t, header{Magic: fileMagic, Version: formatVersion}, sampleEntry())
	sealedGood := sealed(good)

	testCases := []struct {
		name    string
		content []byte
		isFile  bool
		wantErr error
	}{
		{"no file", nil, false, ErrMiss},
		{"garbage", []byte("not a gob stream at all"), true, ErrCorrupt},
		{"empty file", []byte{}, true, ErrCorrupt},
		{"truncated body", good[:len(good)-40], true, ErrCorrupt},
		{"truncated body under a valid digest", sealed(good[:len(good)-40]), true, ErrCorrupt},
		{"a good entry without its digest", good, true, ErrCorrupt},
		{"a good entry with one flipped byte", flipped(sealedGood), true, ErrCorrupt},
		{
			"header only",
			sealed(encoded(t, header{Magic: fileMagic, Version: formatVersion})),
			true,
			ErrCorrupt,
		},
		{
			"wrong magic",
			sealed(encoded(t, header{Magic: "other", Version: formatVersion}, sampleEntry())),
			true,
			ErrCorrupt,
		},
		{
			"older format version",
			sealed(encoded(t, header{Magic: fileMagic, Version: formatVersion - 1})),
			true,
			ErrMiss,
		},
		{
			"newer format version",
			sealed(encoded(t, header{Magic: fileMagic, Version: formatVersion + 1})),
			true,
			ErrMiss,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			store := Open(t.TempDir())
			key := testKey(testCase.name)

			if testCase.isFile {
				path := filepath.Join(store.dir, key+".gob")
				if err := os.WriteFile(path, testCase.content, 0o600); err != nil {
					t.Fatalf("seed the entry: %v", err)
				}
			}

			if _, err := store.Load(key); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Load() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestStoreRejectsBadKeys(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())

	testCases := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"traversal", "../../etc/passwd"},
		{"short hex", "abcd"},
		{"not hex", strings.Repeat("z", 64)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := store.Save(testCase.key, Entry{}); !errors.Is(err, ErrBadKey) {
				t.Fatalf("Save() err = %v, want ErrBadKey", err)
			}

			if _, err := store.Load(testCase.key); !errors.Is(err, ErrBadKey) {
				t.Fatalf("Load() err = %v, want ErrBadKey", err)
			}
		})
	}
}

func TestStoreSaveFailsUnderAFile(t *testing.T) {
	t.Parallel()

	blocker := writeFile(t, t.TempDir(), "blocker", "x", 0o600)
	store := Open(filepath.Join(blocker, "cache"))

	if err := store.Save(testKey("blocked"), sampleEntry()); err == nil {
		t.Fatal("Save() err = nil, want the failure to create the cache dir")
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/cache-root")

	got, err := DefaultDir()
	if err != nil || got != "/cache-root/redigit/plancache" {
		t.Fatalf("DefaultDir() = %q, %v", got, err)
	}
}

func syntheticEntry(decls, edges int) Entry {
	entry := Entry{Ordering: entities.Ordering{
		At: make(map[entities.DeclKey]entities.Position, decls), Rounds: 4,
	}}

	for idx := range decls {
		key := entities.DeclKey{
			File: fmt.Sprintf("pkg%d/file%d.go", idx%50, idx), Kind: entities.KindFunc,
			Name: fmt.Sprintf("Decl%d", idx),
		}
		entry.Decls = append(
			entry.Decls,
			entities.Decl{Key: key, Unit: entities.UnitID(key.File), Lines: 9},
		)
		entry.Ordering.At[key] = entities.Position(idx)
	}

	for idx := range edges {
		entry.Edges = append(entry.Edges, entities.Edge{
			Dependent: entry.Decls[idx%decls].Key,
			Provider:  entry.Decls[(idx*7+1)%decls].Key,
			Reason:    entities.ReasonReference,
		})
	}

	return entry
}

func BenchmarkSaveLoad(b *testing.B) {
	entry := syntheticEntry(2000, 20000)
	store := Open(b.TempDir())
	key := testKey("bench")

	b.Run("save", func(b *testing.B) {
		b.ReportAllocs()

		for range b.N {
			if err := store.Save(key, entry); err != nil {
				b.Fatal(err)
			}
		}

		stat, _ := os.Stat(filepath.Join(store.dir, key+".gob"))
		b.ReportMetric(float64(stat.Size()), "entry-bytes")
	})

	b.Run("load", func(b *testing.B) {
		b.ReportAllocs()

		for range b.N {
			if _, err := store.Load(key); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func sealed(payload []byte) []byte {
	sum := sha256.Sum256(payload)

	return append(slices.Clone(payload), sum[:]...)
}

func flipped(content []byte) []byte {
	out := slices.Clone(content)
	out[len(out)/2] ^= 0x01

	return out
}

func TestStoreDetectsAFlippedByteInAStringField(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	key := testKey("flip")

	if err := store.Save(key, sampleEntry()); err != nil {
		t.Fatalf("Save() err = %v", err)
	}

	path := filepath.Join(store.dir, key+".gob")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the entry: %v", err)
	}

	at := bytes.Index(content, []byte("DRAFT: add"))
	if at < 0 {
		t.Fatal("the saved entry does not hold the plan subject")
	}

	content[at+len("DRAFT: ")] ^= 0x01

	if err = os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("rewrite the entry: %v", err)
	}

	if _, err = store.Load(key); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load() err = %v, want ErrCorrupt rather than a wrong hit", err)
	}
}

func TestStoreSaveLeavesNoTempFileWhenItFails(t *testing.T) {
	t.Parallel()

	store := Open(t.TempDir())
	key := testKey("publish-fails")

	taken := filepath.Join(store.dir, key+".gob")
	if err := os.MkdirAll(filepath.Join(taken, "inner"), 0o700); err != nil {
		t.Fatalf("occupy the entry path: %v", err)
	}

	if err := store.Save(key, sampleEntry()); err == nil {
		t.Fatal("Save() err = nil, want the failure to publish")
	}

	if leftovers, _ := filepath.Glob(filepath.Join(store.dir, "*.tmp")); len(leftovers) != 0 {
		t.Fatalf("Save() left %v behind after failing", leftovers)
	}
}

func TestStoreNeverShowsAHalfWrittenEntry(t *testing.T) {
	t.Parallel()

	const saves = 12

	store := Open(t.TempDir())
	key := testKey("race")
	entry := syntheticEntry(800, 8000)
	start := make(chan struct{})
	done := make(chan struct{})
	seen := make(chan error, 2)

	go func() {
		defer close(done)

		<-start

		for range saves {
			if err := store.Save(key, entry); err != nil {
				seen <- err

				return
			}
		}
	}()

	var readers sync.WaitGroup

	readers.Go(func() {
		<-start

		for {
			select {
			case <-done:
				return
			default:
			}

			if _, err := store.Load(key); err != nil && !errors.Is(err, ErrMiss) {
				seen <- err

				return
			}
		}
	})

	close(start)
	readers.Wait()
	<-done

	select {
	case err := <-seen:
		t.Fatalf("a reader saw %v while the entry was being rewritten", err)
	default:
	}
}

func TestSaveKeepsTheCacheDirPrivate(t *testing.T) {
	t.Parallel()

	loose := t.TempDir()
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatalf("loosen the dir: %v", err)
	}

	testCases := []struct {
		name string
		dir  string
	}{
		{"a dir Save creates", filepath.Join(t.TempDir(), "made", "cache")},
		{"an existing dir left looser", loose},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := Open(testCase.dir).Save(testKey("private"), sampleEntry()); err != nil {
				t.Fatalf("Save() err = %v", err)
			}

			stat, err := os.Stat(testCase.dir)
			if err != nil {
				t.Fatalf("Stat() err = %v", err)
			}

			if stat.Mode().Perm() != 0o700 {
				t.Fatalf("cache dir mode = %v, want 0700", stat.Mode().Perm())
			}
		})
	}
}
