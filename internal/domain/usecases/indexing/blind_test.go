package indexing

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func manyDecls(count int) []entities.Decl {
	out := make([]entities.Decl, 0, count)
	for index := range count {
		out = append(out, entities.Decl{
			Key: entities.DeclKey{
				File: "a.go",
				Kind: entities.KindFunc,
				Name: "F" + string(rune('A'+index)),
			},
			Unit: "a",
		})
	}

	return out
}

func indexWith(
	t *testing.T,
	decls []entities.Decl,
	refsErr error,
) (Result, error) {
	t.Helper()

	ctrl := gomock.NewController(t)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil).AnyTimes()

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(decls, nil).
		AnyTimes()

	resolver := NewMockResolver(ctrl)
	resolver.EXPECT().
		References(gomock.Any(), gomock.Any()).
		Return(nil, refsErr).
		AnyTimes()
	resolver.EXPECT().
		Implementations(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	return New(resolver, tree, Config{
		Concurrency: 1,
		Languages:   []Language{{Name: "go", Extensions: []string{".go"}, Syntax: syntax}},
	}).Index(context.Background(), entities.Snapshot{Files: []string{"a.go"}})
}

// TestIndexRefusesWhenBlindnessDominates pins the line between tolerating a
// server's individual refusals and handing back a graph built from almost
// nothing. A graph missing most of its edges looks exactly like code with
// almost no edges, and nothing downstream can tell the difference.
func TestIndexRefusesWhenBlindnessDominates(t *testing.T) {
	t.Parallel()

	_, err := indexWith(t, manyDecls(10), errors.New("no package metadata"))
	if !errors.Is(err, apperr.ErrUnresolved) {
		t.Fatalf("Index() = %v, want %v", err, apperr.ErrUnresolved)
	}
}

func TestIndexToleratesAFewRefusals(t *testing.T) {
	t.Parallel()

	result, err := indexWith(t, manyDecls(10), nil)
	if err != nil {
		t.Fatalf("Index() = %v", err)
	}

	if len(result.Blind) != 0 {
		t.Fatalf("Blind = %v, want none", result.Blind)
	}
}

func TestResultBlindRatio(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		decls int
		blind int
		want  float64
	}{
		{"nothing indexed divides by nothing", 0, 0, 0},
		{"none blind", 4, 0, 0},
		{"a quarter blind", 4, 1, 0.25},
		{"all blind", 4, 4, 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			blind := make([]Blind, testCase.blind)
			result := Result{
				Graph: entities.NewGraph(manyDecls(testCase.decls), nil, nil),
				Blind: blind,
			}

			if got := result.BlindRatio(); got != testCase.want {
				t.Fatalf("BlindRatio() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestImplementationsAskedOnlyOfInterfaces pins the direction of an
// implementation edge. A server answers the question both ways; keeping the
// answer for a concrete type would emit an edge saying the interface depends
// on whatever satisfies it, which is backwards and puts a stable library
// behind a test stub in another module.
func TestImplementationsAskedOnlyOfInterfaces(t *testing.T) {
	t.Parallel()

	port := entities.Decl{
		Key:         entities.DeclKey{File: "a.go", Kind: entities.KindType, Name: "Reader"},
		Unit:        "a",
		Selector:    entities.Location{File: "a.go", Line: 2, Char: 5},
		IsInterface: true,
	}
	impl := entities.Decl{
		Key:      entities.DeclKey{File: "b.go", Kind: entities.KindType, Name: "Repo"},
		Unit:     "b",
		Selector: entities.Location{File: "b.go", Line: 2, Char: 5},
	}

	ctrl := gomock.NewController(t)

	resolver := NewMockResolver(ctrl)
	resolver.EXPECT().References(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	// Only the interface may be asked. Any call for the concrete type fails
	// the expectation, which is the point of the test.
	resolver.EXPECT().
		Implementations(gomock.Any(), port.Selector).
		Return(nil, nil).
		Times(1)

	indexer := New(resolver, nil, Config{Concurrency: 1})

	_, blind, err := indexer.resolve(
		context.Background(),
		[]entities.Decl{port, impl},
		newLocator(),
	)
	if err != nil {
		t.Fatalf("resolve() = %v", err)
	}

	if len(blind) != 0 {
		t.Fatalf("blind = %v, want none", blind)
	}
}
