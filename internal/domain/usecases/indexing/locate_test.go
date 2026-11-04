package indexing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestLocatorEnclosing(t *testing.T) {
	t.Parallel()

	source := []byte(
		"package a\n\nimport \"fmt\"\n\nfunc One() {\n\tfmt.Print()\n}\n\nfunc Two() {}\n",
	)
	one := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "One"}
	two := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Two"}

	where := newLocator()
	where.add("a.go", source, []entities.Decl{
		{Key: two, Span: entities.Span{Start: 55, End: 69}},
		{Key: one, Span: entities.Span{Start: 25, End: 54}},
	})

	testCases := []struct {
		name      string
		at        entities.Location
		wantKey   entities.DeclKey
		wantFound bool
	}{
		{
			"inside the first declaration",
			entities.Location{File: "a.go", Line: 5, Char: 2},
			one,
			true,
		},
		{"inside the second", entities.Location{File: "a.go", Line: 8, Char: 5}, two, true},
		{
			"the import block belongs to no declaration, which is an answer not a failure",
			entities.Location{File: "a.go", Line: 2, Char: 8},
			entities.DeclKey{},
			false,
		},
		{
			"an unknown file",
			entities.Location{File: "b.go", Line: 0, Char: 0},
			entities.DeclKey{},
			false,
		},
		{
			"a line past the file",
			entities.Location{File: "a.go", Line: 99, Char: 0},
			entities.DeclKey{},
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			key, found := where.Enclosing(testCase.at)
			if found != testCase.wantFound {
				t.Fatalf("Enclosing() found = %v, want %v", found, testCase.wantFound)
			}

			if key != testCase.wantKey {
				t.Fatalf("Enclosing() = %v, want %v", key, testCase.wantKey)
			}
		})
	}
}

// TestLocatorGapBetweenDeclarations pins that a location in the blank space
// after a declaration ends is not attributed to it. Reading one byte past a
// span as still inside it would invent an edge from whatever sits in the gap.
func TestLocatorGapBetweenDeclarations(t *testing.T) {
	t.Parallel()

	source := []byte("package a\n\nfunc A() {}\n\n// a loose comment\n\nfunc B() {}\n")
	where := newLocator()
	where.add("a.go", source, []entities.Decl{
		{
			Key:  entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "A"},
			Span: entities.Span{Start: 11, End: 22},
		},
	})

	if _, found := where.Enclosing(entities.Location{File: "a.go", Line: 4, Char: 3}); found {
		t.Fatal("a location in the gap after a declaration was attributed to it")
	}
}

func TestReattachGivesEachReaderToTheStatementThatWroteIt(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	indexer, cut, where, graph := cutRun(t)

	resolver := NewMockResolver(ctrl)
	resolver.EXPECT().
		References(gomock.Any(), entities.Location{File: "a.go", Line: 7, Char: 5}).
		Return([]entities.Location{{File: "a.go", Line: 3, Char: 1}}, nil)
	resolver.EXPECT().
		References(gomock.Any(), entities.Location{File: "a.go", Line: 9, Char: 5}).
		Return([]entities.Location{{File: "a.go", Line: 4, Char: 1}}, nil)
	resolver.EXPECT().Implementations(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	indexer.resolver = resolver

	got := indexer.reattach(context.Background(), graph, cut, where)

	want := []entities.Edge{
		{
			Dependent: sliceKey(entities.KindStmt, "Run#0"),
			Provider:  sliceKey(entities.KindFunc, "One"),
			Reason:    entities.ReasonReference,
		},
		{
			Dependent: sliceKey(entities.KindStmt, "Run#1"),
			Provider:  sliceKey(entities.KindFunc, "Two"),
			Reason:    entities.ReasonReference,
		},
	}

	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("reattach() = %v, want %v", got, want)
	}
}

func TestCutWiringRefusesAGlobThatCutsNothing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		pick     selection
		wantDead []string
	}{
		{
			name:     "a glob matching nothing is reported",
			pick:     selection{globs: []string{"Run", "Nothing*"}, kind: entities.KindFunc},
			wantDead: []string{"Nothing*"},
		},
		{
			name: "a generated function is never cut so its glob is dead",
			pick: selection{
				globs: []string{"Run"}, kind: entities.KindFunc, generated: []string{"Run"},
			},
			wantDead: []string{"Run"},
		},
		{
			name: "a test-side function is never cut so its glob is dead",
			pick: selection{
				globs: []string{"One"}, kind: entities.KindFunc, testSide: []string{"One"},
			},
			wantDead: []string{"One"},
		},
		{
			name: "a function the slicer refuses is not cut so its glob is dead",
			pick: selection{
				globs: []string{"Run"}, kind: entities.KindFunc, refused: []string{"Run"},
			},
			wantDead: []string{"Run"},
		},
		{
			name: "a glob whose every match the slicer refuses is dead",
			pick: selection{
				globs: []string{"Run", "O*"}, kind: entities.KindFunc, refused: []string{"One"},
			},
			wantDead: []string{"O*"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := selectedCut(t, testCase.pick)
			if !errors.Is(err, ErrDeadGlob) {
				t.Fatalf("cutWiring() error = %v, want %v", err, ErrDeadGlob)
			}

			for _, glob := range testCase.wantDead {
				if !strings.Contains(err.Error(), glob) {
					t.Fatalf("cutWiring() error = %q, want it to name %q", err, glob)
				}
			}
		})
	}
}
