package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/chunk"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

func TestChunkersPrependsTheMapApplierWhenGiven(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		chunkMap entities.ChunkMap
		wantHead bool
	}{
		{"no chunk map, nothing prepended", nil, false},
		{
			"a chunk map is prepended first",
			entities.ChunkMap{"a.ts": {{Name: "x", Span: entities.Span{Start: 0, End: 1}}}},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out := chunkers(testCase.chunkMap)
			_, headIsMapApplier := out[0].(chunk.MapApplier)

			if headIsMapApplier != testCase.wantHead {
				t.Fatalf(
					"chunkers()[0] is a MapApplier = %v, want %v",
					headIsMapApplier,
					testCase.wantHead,
				)
			}

			for _, chunker := range out {
				if _, isWholeFile := chunker.(chunk.WholeFile); isWholeFile {
					t.Fatal("chunkers() must never include WholeFile: it claims every path")
				}
			}
		})
	}
}

func TestConfigChunkersPrependsTheMapApplierWhenGiven(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		chunkMap entities.ChunkMap
		wantHead bool
	}{
		{"no chunk map, nothing prepended", nil, false},
		{
			"a chunk map is prepended first",
			entities.ChunkMap{"a.ts": {{Name: "x", Span: entities.Span{Start: 0, End: 1}}}},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out := configChunkers(testCase.chunkMap)
			_, headIsMapApplier := out[0].(chunk.MapApplier)

			if headIsMapApplier != testCase.wantHead {
				t.Fatalf(
					"configChunkers()[0] is a MapApplier = %v, want %v",
					headIsMapApplier,
					testCase.wantHead,
				)
			}

			if _, isWholeFile := out[len(out)-1].(chunk.WholeFile); !isWholeFile {
				t.Fatal(
					"configChunkers() must end with WholeFile: an owned file no chunker claims still has to reach the tree",
				)
			}
		})
	}
}

func TestValidateChunkMap(t *testing.T) {
	t.Parallel()

	specific := specificLanguages(
		config.NewSyntax(configChunkers(nil)),
		finalTree{},
		typed.NewSource(nil),
		sliceconf.Config{},
	)
	snap := entities.Snapshot{Files: []string{"main.go", "go.mod", "engines/wwebjs/src/main.tmpl"}}

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
				"engines/wwebjs/src/main.tmpl": {
					{Name: "x", Span: entities.Span{Start: 0, End: 1}},
				},
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
