package inputs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestParseChunkMap(t *testing.T) {
	t.Parallel()

	const content = `{"database/queries/lead.sql": [
		{"name": "GetLeadByID", "span": [0, 240]},
		{"name": "ListLeadsByOrg", "span": [241, 520]}
	]}`

	got, err := ParseChunkMap([]byte(content))
	if err != nil {
		t.Fatalf("ParseChunkMap() = %v", err)
	}

	spans := got["database/queries/lead.sql"]
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2", len(spans))
	}

	want := entities.ChunkSpan{Name: "GetLeadByID", Span: entities.Span{Start: 0, End: 240}}
	if spans[0] != want {
		t.Fatalf("spans[0] = %+v, want %+v", spans[0], want)
	}
}

func TestParseChunkMapRefusesMalformedJSON(t *testing.T) {
	t.Parallel()

	_, err := ParseChunkMap([]byte("not json"))
	if !errors.Is(err, ErrMalformedChunkMap) {
		t.Fatalf("ParseChunkMap() err = %v, want ErrMalformedChunkMap", err)
	}
}

func TestLoadChunkMapReadsAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "chunkmap.json")
	content := `{"a.ts": [{"name": "piece", "span": [0, 3]}]}`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	chunkMap, err := LoadChunkMap(path)
	if err != nil {
		t.Fatalf("LoadChunkMap() = %v", err)
	}

	if len(chunkMap["a.ts"]) != 1 {
		t.Fatalf("chunkMap[a.ts] = %v, want 1 span", chunkMap["a.ts"])
	}
}

func TestLoadChunkMapFailsOnAMissingFile(t *testing.T) {
	t.Parallel()

	_, err := LoadChunkMap(filepath.Join(t.TempDir(), "absent.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadChunkMap() err = %v, want it to wrap os.ErrNotExist", err)
	}
}
