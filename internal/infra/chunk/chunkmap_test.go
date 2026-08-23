package chunk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestMapApplierHandles(t *testing.T) {
	t.Parallel()

	applier := MapApplier{
		Map: entities.ChunkMap{"a.txt": {{Name: "x", Span: entities.Span{Start: 0, End: 1}}}},
	}

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{"path in map", "a.txt", true},
		{"path not in map", "b.txt", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := applier.Handles(testCase.path); got != testCase.want {
				t.Fatalf("Handles(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestMapApplierChunks(t *testing.T) {
	t.Parallel()

	const source = "0123456789"

	testCases := []struct {
		name      string
		spans     []entities.ChunkSpan
		wantNames []string
	}{
		{
			name: "full coverage, no gap",
			spans: []entities.ChunkSpan{
				{Name: "first", Span: entities.Span{Start: 0, End: 5}},
				{Name: "second", Span: entities.Span{Start: 5, End: 10}},
			},
			wantNames: []string{"first", "second"},
		},
		{
			name: "a gap between two spans becomes its own piece",
			spans: []entities.ChunkSpan{
				{Name: "first", Span: entities.Span{Start: 0, End: 3}},
				{Name: "second", Span: entities.Span{Start: 7, End: 10}},
			},
			wantNames: []string{"first", "<gap:3-7>", "second"},
		},
		{
			name: "a gap before the first span and after the last",
			spans: []entities.ChunkSpan{
				{Name: "middle", Span: entities.Span{Start: 3, End: 7}},
			},
			wantNames: []string{"<gap:0-3>", "middle", "<gap:7-10>"},
		},
		{
			name:      "empty map leaves the whole file as one gap",
			spans:     nil,
			wantNames: []string{"<gap:0-10>"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			applier := MapApplier{Map: entities.ChunkMap{"x": testCase.spans}}

			decls, err := applier.Chunks(context.Background(), "x", []byte(source))
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			assertReconstructs(t, source, decls)
			assertDeclNames(t, decls, testCase.wantNames)
		})
	}
}

func TestMapApplierChunksEmptyFile(t *testing.T) {
	t.Parallel()

	applier := MapApplier{Map: entities.ChunkMap{"x": nil}}

	decls, err := applier.Chunks(context.Background(), "x", []byte(""))
	if err != nil {
		t.Fatalf("Chunks() error = %v", err)
	}

	if len(decls) != 0 {
		t.Fatalf("Chunks() = %d decls, want 0 for an empty file with no spans", len(decls))
	}
}

func TestMapApplierChunksOverlapError(t *testing.T) {
	t.Parallel()

	applier := MapApplier{
		Map: entities.ChunkMap{
			"x": {
				{Name: "first", Span: entities.Span{Start: 0, End: 6}},
				{Name: "second", Span: entities.Span{Start: 4, End: 10}},
			},
		},
	}

	_, err := applier.Chunks(context.Background(), "x", []byte("0123456789"))
	if !errors.Is(err, ErrOverlappingSpans) {
		t.Fatalf("err = %v, want %v", err, ErrOverlappingSpans)
	}

	for _, name := range []string{"first", "second"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error %q does not name overlapping span %q", err, name)
		}
	}
}

func TestMapApplierChunksOutOfRangeError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		span entities.Span
	}{
		{"end past file", entities.Span{Start: 0, End: 999}},
		{"negative start", entities.Span{Start: -1, End: 5}},
		{"start after end", entities.Span{Start: 5, End: 2}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			applier := MapApplier{
				Map: entities.ChunkMap{"x": {{Name: "bad", Span: testCase.span}}},
			}

			_, err := applier.Chunks(context.Background(), "x", []byte("0123456789"))
			if !errors.Is(err, ErrSpanOutOfRange) {
				t.Fatalf("err = %v, want %v", err, ErrSpanOutOfRange)
			}
		})
	}
}
