package indexing

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestIndexerEdgesFrom(t *testing.T) {
	t.Parallel()

	source := []byte("package a\n\nfunc One() {\n\tTwo()\n}\n\nfunc Two() {}\n")
	one := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "One"}
	two := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Two"}

	where := newLocator()
	where.add("a.go", source, []entities.Decl{
		{Key: one, Span: entities.Span{Start: 11, End: 32}},
		{Key: two, Span: entities.Span{Start: 34, End: 48}},
	})

	indexer := Indexer{}

	testCases := []struct {
		name  string
		found []entities.Location
		want  int
	}{
		{
			"a reader in another declaration becomes an edge",
			[]entities.Location{{File: "a.go", Line: 3, Char: 1}},
			1,
		},
		{
			"a mention inside the provider itself is not a reader; nothing waits for itself",
			[]entities.Location{{File: "a.go", Line: 6, Char: 5}},
			0,
		},
		{
			"a location in no declaration is dropped rather than guessed at",
			[]entities.Location{{File: "a.go", Line: 0, Char: 0}},
			0,
		},
		{"an unknown file", []entities.Location{{File: "z.go", Line: 0, Char: 0}}, 0},
		{"nothing found", nil, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := indexer.edgesFrom(testCase.found, two, entities.ReasonReference, where)
			if len(got) != testCase.want {
				t.Fatalf("edgesFrom() = %v, want %d edges", got, testCase.want)
			}
		})
	}
}

func TestIndexerResolveDeduplicates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := []byte("package a\n\nfunc One() {\n\tTwo()\n\t_ = Two\n}\n\nfunc Two() {}\n")
	one := entities.Decl{
		Key:  entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "One"},
		Span: entities.Span{Start: 11, End: 39},
	}
	two := entities.Decl{
		Key:      entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Two"},
		Span:     entities.Span{Start: 41, End: 55},
		Selector: entities.Location{File: "a.go", Line: 7, Char: 5},
	}

	where := newLocator()
	where.add("a.go", source, []entities.Decl{one, two})

	resolver := NewMockResolver(ctrl)
	// The same reader calling twice is one dependency, not two.
	resolver.EXPECT().References(gomock.Any(), two.Selector).Return([]entities.Location{
		{File: "a.go", Line: 3, Char: 1},
		{File: "a.go", Line: 4, Char: 1},
	}, nil)
	resolver.EXPECT().References(gomock.Any(), one.Selector).Return(nil, nil)
	resolver.EXPECT().Implementations(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	indexer := New(resolver, nil, Config{Concurrency: 1})

	edges, blind, err := indexer.resolve(context.Background(), []entities.Decl{one, two}, where)
	if err != nil {
		t.Fatalf("resolve() = %v", err)
	}

	if len(blind) != 0 {
		t.Fatalf("blind = %v, want none", blind)
	}

	if len(edges) != 1 {
		t.Fatalf("resolve() = %v, want a single deduplicated edge", edges)
	}

	if edges[0].Dependent != one.Key || edges[0].Provider != two.Key {
		t.Fatalf("edge = %v, want %s needing %s", edges[0], one.Key, two.Key)
	}
}

func TestCompareEdges(t *testing.T) {
	t.Parallel()

	base := entities.Edge{
		Dependent: entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "A"},
		Provider:  entities.DeclKey{File: "b.go", Kind: entities.KindFunc, Name: "B"},
		Reason:    entities.ReasonReference,
	}

	other := base
	other.Reason = entities.ReasonImplements

	testCases := []struct {
		name string
		left entities.Edge
		want bool
	}{
		{"identical edges compare equal", base, true},
		{"the same pair with a different reason is a different edge", other, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if equal := compareEdges(base, testCase.left) == 0; equal != testCase.want {
				t.Fatalf("compareEdges() equal = %v, want %v", equal, testCase.want)
			}
		})
	}
}
