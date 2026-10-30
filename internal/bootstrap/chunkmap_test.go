package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

func TestValidateChunkMap(t *testing.T) {
	t.Parallel()

	specific := specificLanguages(
		config.NewSyntax(configChunkers(nil)), finalTree{}, typed.NewSource(nil),
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
