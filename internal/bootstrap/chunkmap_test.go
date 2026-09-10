package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
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

func TestValidateChunkMap(t *testing.T) {
	t.Parallel()

	specific := specificLanguages(config.NewSyntax(configChunkers(nil)))
	snap := entities.Snapshot{Files: []string{"main.go", "go.mod", "engines/wwebjs/src/main.ts"}}

	testCases := []struct {
		name      string
		chunkMap  entities.ChunkMap
		wantErr   error
		wantOwner string
	}{
		{name: "an empty map is always fine"},
		{
			name: "a path the opaque pack would otherwise own is fine",
			chunkMap: entities.ChunkMap{
				"engines/wwebjs/src/main.ts": {{Name: "x", Span: entities.Span{Start: 0, End: 1}}},
			},
		},
		{
			name: "a path outside the tree is refused",
			chunkMap: entities.ChunkMap{
				"does/not/exist.ts": {{Name: "x", Span: entities.Span{Start: 0, End: 1}}},
			},
			wantErr: ErrChunkMapUnknownPath,
		},
		{
			name: "a path the go pack already claims is refused",
			chunkMap: entities.ChunkMap{
				"main.go": {{Name: "x", Span: entities.Span{Start: 0, End: 1}}},
			},
			wantErr:   ErrChunkMapConflict,
			wantOwner: "go",
		},
		{
			name: "a path the config pack already claims is refused",
			chunkMap: entities.ChunkMap{
				"go.mod": {{Name: "x", Span: entities.Span{Start: 0, End: 1}}},
			},
			wantErr:   ErrChunkMapConflict,
			wantOwner: "config",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := validateChunkMap(testCase.chunkMap, snap, specific)

			if testCase.wantErr == nil {
				if err != nil {
					t.Fatalf("validateChunkMap() = %v, want nil", err)
				}

				return
			}

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("validateChunkMap() = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantOwner != "" && !strings.Contains(err.Error(), testCase.wantOwner) {
				t.Fatalf("error %q does not name the owning pack %q", err, testCase.wantOwner)
			}
		})
	}
}
