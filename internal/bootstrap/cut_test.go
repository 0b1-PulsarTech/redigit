package bootstrap

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/chunk"
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
