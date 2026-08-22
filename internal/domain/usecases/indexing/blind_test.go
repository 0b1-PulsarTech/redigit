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
