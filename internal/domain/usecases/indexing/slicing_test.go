package indexing

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// cuttingSyntax answers every Slice with the same pieces, so the test can ask
// what the indexer does with them instead of how a language cuts.
type cuttingSyntax struct {
	*MockSyntax

	sliced Sliced
}

func (s cuttingSyntax) Slice(
	context.Context,
	entities.Decl,
	[]byte,
) (Sliced, bool, error) {
	return s.sliced, true, nil
}

const sliceSource = "package a\n\nfunc Run() {\n\tOne()\n\tTwo()\n}\n\nfunc One() {}\n\nfunc Two() {}\n"

func sliceKey(kind entities.DeclKind, name string) entities.DeclKey {
	return entities.DeclKey{File: "a.go", Kind: kind, Name: name}
}

func cutRun(t *testing.T) (Indexer, wiringCut, locator, entities.Graph) {
	t.Helper()

	ctrl := gomock.NewController(t)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), "a.go").Return([]byte(sliceSource), nil).AnyTimes()

	run := entities.Decl{
		Key:  sliceKey(entities.KindFunc, "Run"),
		Span: entities.Span{Start: 11, End: 39},
	}
	one := entities.Decl{
		Key:      sliceKey(entities.KindFunc, "One"),
		Span:     entities.Span{Start: 41, End: 54},
		Selector: entities.Location{File: "a.go", Line: 7, Char: 5},
	}
	two := entities.Decl{
		Key:      sliceKey(entities.KindFunc, "Two"),
		Span:     entities.Span{Start: 56, End: 69},
		Selector: entities.Location{File: "a.go", Line: 9, Char: 5},
	}

	head := run
	head.Span.End = 23

	first := entities.Decl{
		Key:  sliceKey(entities.KindStmt, "Run#0"),
		Span: entities.Span{Start: 25, End: 30},
	}
	second := entities.Decl{
		Key:  sliceKey(entities.KindStmt, "Run#1"),
		Span: entities.Span{Start: 32, End: 37},
	}

	syntax := cuttingSyntax{
		MockSyntax: NewMockSyntax(ctrl),
		sliced:     Sliced{Head: head, Members: []entities.Decl{first, second}},
	}

	indexer := New(
		quietResolver(ctrl),
		tree,
		Config{Languages: []Language{{
			Name: "go", Extensions: []string{".go"}, Syntax: syntax, Resolved: true,
		}}},
	)

	decls := []entities.Decl{run, one, two}
	where := newLocator()
	where.add("a.go", []byte(sliceSource), decls)

	edges := []entities.Edge{
		{Dependent: run.Key, Provider: one.Key, Reason: entities.ReasonReference},
		{Dependent: run.Key, Provider: two.Key, Reason: entities.ReasonReference},
	}
	graph := entities.NewGraph(decls, edges, nil)

	cut := wiringCut{replaced: map[entities.DeclKey]bool{}, added: map[entities.DeclKey]bool{}}
	byFile := map[string][]entities.Decl{"a.go": decls}

	err := indexer.cutFile(
		context.Background(),
		"a.go",
		byFile,
		map[entities.DeclKey]bool{run.Key: true},
		&cut,
		where,
	)
	if err != nil {
		t.Fatalf("cutFile() = %v", err)
	}

	cut.decls = byFile["a.go"]

	return indexer, cut, where, graph
}

func TestCutFileReplacesAWiringFunctionWithItsPieces(t *testing.T) {
	t.Parallel()

	_, cut, where, _ := cutRun(t)

	if !cut.replaced[sliceKey(entities.KindFunc, "Run")] {
		t.Fatalf("replaced = %v, want Run", cut.replaced)
	}

	if len(cut.decls) != 5 || len(cut.added) != 3 {
		t.Fatalf("decls = %d, added = %d, want the head and two statements besides One and Two",
			len(cut.decls), len(cut.added))
	}

	// A reader on the second line of the body now belongs to the statement
	// written there, not to the whole function.
	at, found := where.Enclosing(entities.Location{File: "a.go", Line: 4, Char: 1})
	if !found || at != sliceKey(entities.KindStmt, "Run#1") {
		t.Fatalf("Enclosing() = %v, %v, want the second statement", at, found)
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
