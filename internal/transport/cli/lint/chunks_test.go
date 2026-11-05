package lint

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/stmtread"
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func chunkRow(position int, keys ...string) manifest.Row {
	row := manifest.Row{Position: position, Subject: "row"}
	for _, key := range keys {
		row.Chunks = append(row.Chunks, manifest.Chunk{Key: key})
	}

	return row
}

func TestMeasureGraphRefusesAManifestTheGraphDoesNotMatch(t *testing.T) {
	t.Parallel()

	head := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "run"}
	first := entities.DeclKey{File: "a.go", Kind: entities.KindStmt, Name: "run#0"}
	second := entities.DeclKey{File: "a.go", Kind: entities.KindStmt, Name: "run#1"}

	graph := entities.NewGraph(
		[]entities.Decl{{Key: head, Unit: "a"}, {Key: first, Unit: "a"}, {Key: second, Unit: "a"}},
		[]entities.Edge{
			{Dependent: first, Provider: head, Reason: entities.ReasonBody},
			{Dependent: second, Provider: head, Reason: entities.ReasonBody},
		}, nil,
	)

	testCases := []struct {
		name      string
		rows      []manifest.Row
		wantNamed []string
	}{
		{
			name:      "a chunk the graph does not know",
			rows:      []manifest.Row{chunkRow(0, "a.go:func:run", "b.go:func:Ghost")},
			wantNamed: []string{"b.go:func:Ghost"},
		},
		{
			name:      "a function the manifest emitted whole and the graph holds cut",
			rows:      []manifest.Row{chunkRow(0, "a.go:func:run")},
			wantNamed: []string{"a.go:func:run"},
		},
		{
			name: "a function the manifest cut and the graph holds without one piece",
			rows: []manifest.Row{
				chunkRow(0, "a.go:func:run", "a.go:stmt:run#0"),
			},
			wantNamed: []string{"a.go:stmt:run#1"},
		},
		{
			name:      "a manifest whose rows hold no chunk at all",
			rows:      []manifest.Row{chunkRow(0), chunkRow(1)},
			wantNamed: []string{"a.go:func:run"},
		},
		{
			name: "a chunk repeated says it is repeated",
			rows: []manifest.Row{
				chunkRow(0, "a.go:func:run", "a.go:stmt:run#0", "a.go:stmt:run#1"),
				chunkRow(1, "a.go:stmt:run#1"),
			},
			wantNamed: []string{"a.go:stmt:run#1", "repeated"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := measureGraph(
				context.Background(), graph, testCase.rows, stmtread.Reader{},
				linting.DefaultLimits(),
			)
			if !errors.Is(err, write.ErrUsage) || !errors.Is(err, linting.ErrMismatch) {
				t.Fatalf("measureGraph() err = %v, want a usage mismatch", err)
			}

			for _, named := range testCase.wantNamed {
				if !strings.Contains(err.Error(), named) {
					t.Fatalf("measureGraph() err = %v, want it to say %q", err, named)
				}
			}
		})
	}
}

func TestBornRowsTellsTwoDeclarationsOfOneKeyTextApartBySpan(t *testing.T) {
	t.Parallel()

	first := entities.DeclKey{File: "x", Kind: entities.KindFunc, Name: "type:T"}
	second := entities.DeclKey{File: "x:func", Kind: entities.KindType, Name: "T"}
	graph := entities.NewGraph([]entities.Decl{
		{Key: first, Unit: "u", Span: entities.Span{Start: 0, End: 5}},
		{Key: second, Unit: "u", Span: entities.Span{Start: 10, End: 20}},
	}, nil, nil)

	chunk := func(start, end int) manifest.Chunk {
		return manifest.Chunk{Key: "x:func:type:T", Span: [2]int{start, end}, Unit: "u"}
	}

	got, err := bornRows(graph, []manifest.Row{
		{Position: 3, Chunks: []manifest.Chunk{chunk(10, 20)}},
		{Position: 7, Chunks: []manifest.Chunk{chunk(0, 5)}},
	})
	if err != nil || got[second] != 3 || got[first] != 7 {
		t.Fatalf("bornRows() = %v, %v, want the span to pick each declaration", got, err)
	}
}
