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

// TestIdenticalInterfacesDoNotDependOnEachOther pins the artifact that turned
// eleven groups of unrelated ports into cycles. A house style that declares
// every port at the consumer ends up with the same small interface written in
// a dozen packages; each satisfies all the others by method set alone, and
// keeping those edges makes every copy appear to depend on every other.
func TestIdenticalInterfacesDoNotDependOnEachOther(t *testing.T) {
	t.Parallel()

	first := entities.Decl{
		Key:         entities.DeclKey{File: "a/port.go", Kind: entities.KindType, Name: "Coder"},
		Unit:        "a",
		Span:        entities.Span{Start: 0, End: 40},
		Selector:    entities.Location{File: "a/port.go", Line: 0, Char: 5},
		IsInterface: true,
	}
	second := entities.Decl{
		Key:         entities.DeclKey{File: "b/port.go", Kind: entities.KindType, Name: "Coder"},
		Unit:        "b",
		Span:        entities.Span{Start: 0, End: 40},
		Selector:    entities.Location{File: "b/port.go", Line: 0, Char: 5},
		IsInterface: true,
	}
	impl := entities.Decl{
		Key:      entities.DeclKey{File: "c/repo.go", Kind: entities.KindType, Name: "Repo"},
		Unit:     "c",
		Span:     entities.Span{Start: 0, End: 40},
		Selector: entities.Location{File: "c/repo.go", Line: 0, Char: 5},
	}

	where := newLocator()
	for _, decl := range []entities.Decl{first, second, impl} {
		where.add(decl.Key.File, []byte("package x\n\ntype T interface{}\n"), []entities.Decl{decl})
	}

	ctrl := gomock.NewController(t)

	resolver := NewMockResolver(ctrl)
	resolver.EXPECT().References(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	// A server answering about one port names the identical port next door
	// alongside the concrete type, because both satisfy it.
	resolver.EXPECT().
		Implementations(gomock.Any(), first.Selector).
		Return([]entities.Location{second.Selector, impl.Selector}, nil)
	resolver.EXPECT().
		Implementations(gomock.Any(), second.Selector).
		Return([]entities.Location{first.Selector, impl.Selector}, nil)

	edges, _, err := New(resolver, nil, Config{Concurrency: 1}).
		resolve(context.Background(), []entities.Decl{first, second, impl}, where)
	if err != nil {
		t.Fatalf("resolve() = %v", err)
	}

	for _, edge := range edges {
		if edge.Dependent == first.Key || edge.Dependent == second.Key {
			t.Fatalf("an interface was made to depend on another: %v", edge)
		}
	}

	if len(edges) != 2 {
		t.Fatalf("edges = %v, want only the concrete type depending on each port", edges)
	}
}

// TestInterfaceDispatchDoesNotLinkSiblings pins the criterion a peer working
// this data proved out: a call written against an interface must not become an
// edge to every concrete method that satisfies it. Seven sibling Error methods
// all appeared to reference each other because one line called e.Err.Error()
// on an error-typed field.
func TestInterfaceDispatchDoesNotLinkSiblings(t *testing.T) {
	t.Parallel()

	method := entities.Decl{
		Key: entities.DeclKey{
			File: "a/err.go",
			Kind: entities.KindMethod,
			Name: "Conflict.Error",
		},
		Unit:     "a",
		Span:     entities.Span{Start: 0, End: 60},
		Selector: entities.Location{File: "a/err.go", Line: 3, Char: 20},
	}

	direct := entities.Location{File: "b/use.go", Line: 9, Char: 4}
	viaInterface := entities.Location{File: "a/other.go", Line: 12, Char: 8}

	ctrl := gomock.NewController(t)

	resolver := NewMockResolver(ctrl)
	// The direct call resolves back to this method.
	resolver.EXPECT().Definition(gomock.Any(), direct).Return(method.Selector, nil)
	// The interface call resolves to the interface's own method, elsewhere.
	resolver.EXPECT().
		Definition(gomock.Any(), viaInterface).
		Return(entities.Location{File: "builtin/error.go", Line: 1, Char: 1}, nil)

	indexer := New(resolver, nil, Config{Concurrency: 1})

	got := indexer.directCallsOnly(context.Background(), method, []entities.Location{
		direct, viaInterface,
	})

	if len(got) != 1 || got[0] != direct {
		t.Fatalf("directCallsOnly() = %v, want only the direct call", got)
	}
}

func TestDirectCallsOnlyLeavesOtherKindsAlone(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	// A resolver with no Definition expectation: calling it would fail the
	// test, which is how "not asked" is asserted.
	resolver := NewMockResolver(ctrl)
	indexer := New(resolver, nil, Config{Concurrency: 1})

	at := entities.Location{File: "b/use.go", Line: 9, Char: 4}

	testCases := []struct {
		name string
		kind entities.DeclKind
	}{
		{"a function is named directly or not at all", entities.KindFunc},
		{"so is a type", entities.KindType},
		{"so is a value", entities.KindValue},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decl := entities.Decl{
				Key: entities.DeclKey{File: "a.go", Kind: testCase.kind, Name: "X"},
			}

			got := indexer.directCallsOnly(context.Background(), decl, []entities.Location{at})
			if len(got) != 1 {
				t.Fatalf("directCallsOnly() = %v, want the reference untouched", got)
			}
		})
	}
}

func TestDirectCallsOnlyKeepsWhatItCannotResolve(t *testing.T) {
	t.Parallel()

	method := entities.Decl{
		Key:      entities.DeclKey{File: "a.go", Kind: entities.KindMethod, Name: "T.Do"},
		Selector: entities.Location{File: "a.go", Line: 1, Char: 1},
	}
	at := entities.Location{File: "b.go", Line: 2, Char: 2}

	ctrl := gomock.NewController(t)

	resolver := NewMockResolver(ctrl)
	resolver.EXPECT().
		Definition(gomock.Any(), at).
		Return(entities.Location{}, errors.New("no definition"))

	got := New(resolver, nil, Config{Concurrency: 1}).
		directCallsOnly(context.Background(), method, []entities.Location{at})

	// An unanswerable definition is not evidence of dispatch. Dropping the
	// edge here would move a declaration ahead of something that does read it.
	if len(got) != 1 {
		t.Fatalf("directCallsOnly() = %v, want the doubtful reference kept", got)
	}
}
