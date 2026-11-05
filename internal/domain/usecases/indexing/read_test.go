package indexing

import (
	"context"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestIndexerReadsOnlyOwnedFiles(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), "a.go").Return([]byte(goSource), nil)

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), "a.go", gomock.Any()).
		Return([]entities.Decl{goDecl("a.go", "One", 11, 24)}, nil)

	result, err := New(
		quietResolver(ctrl),
		tree,
		Config{Languages: []Language{{Name: "go", Extensions: []string{".go"}, Syntax: syntax}}},
	).Index(context.Background(), entities.Snapshot{Files: []string{"a.go", "notes.md", "q.sql"}})
	if err != nil {
		t.Fatalf("Index() = %v", err)
	}

	if result.Graph.Len() != 1 {
		t.Fatalf("graph holds %d declarations, want 1", result.Graph.Len())
	}
}

func twinDecls(files []string) []entities.Decl {
	out := make([]entities.Decl, 0, 5*len(files))

	for fileIndex, file := range files {
		for twin := range 3 {
			start := 100*fileIndex + 10*twin
			out = append(out, goDecl(file, "init", start, start+5))
		}

		out = append(out, goDecl(file, "Run", 100*fileIndex+50, 100*fileIndex+55))
		out = append(out, goDecl(file, "Stop", 100*fileIndex+60, 100*fileIndex+65))
	}

	return out
}

func TestIndexerReadReturnsDeclarationsInAFixedOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		files []string
	}{
		{"four files", []string{"a.go", "b.go", "c.go", "d.go"}},
		{"four files given backwards", []string{"d.go", "c.go", "b.go", "a.go"}},
		{"three files in a nested directory", []string{"p/x.go", "p/y.go", "q/z.go"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			byFile := map[string][]entities.Decl{}

			for _, decl := range twinDecls(testCase.files) {
				byFile[decl.Key.File] = append(byFile[decl.Key.File], decl)
			}

			tree := NewMockTree(ctrl)
			tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil).AnyTimes()

			syntax := NewMockSyntax(ctrl)
			syntax.EXPECT().Declarations(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, file string, _ []byte) ([]entities.Decl, error) {
					runtime.Gosched()

					return slices.Clone(byFile[file]), nil
				},
			).AnyTimes()

			indexer := New(NewMockResolver(ctrl), tree, Config{
				Concurrency: 4,
				Languages:   []Language{{Name: "go", Extensions: []string{".go"}, Syntax: syntax}},
			})

			var first []entities.Decl

			for run := range 64 {
				got, _, err := indexer.read(
					context.Background(), entities.Snapshot{Files: testCase.files},
				)
				if err != nil {
					t.Fatalf("read() = %v", err)
				}

				if run == 0 {
					first = got
				}

				if !reflect.DeepEqual(got, first) {
					t.Fatalf("run %d: read() = %v, want the first run's %v", run, got, first)
				}
			}

			if !slices.IsSortedFunc(first, compareDeclKeys) {
				t.Fatalf("read() = %v, want key order", first)
			}
		})
	}
}
