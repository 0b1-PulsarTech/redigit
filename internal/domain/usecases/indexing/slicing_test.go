package indexing

import (
	"context"
	"reflect"
	"slices"
	"strings"
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

func TestCutWiringReturnsDeclarationsInAFixedOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		files []string
	}{
		{"four files", []string{"a.go", "b.go", "c.go", "d.go"}},
		{"four files given backwards", []string{"d.go", "c.go", "b.go", "a.go"}},
		{"three files in a nested directory", []string{"p/x.go", "p/y.go", "q/z.go"}},
		{"five files", []string{"e.go", "a.go", "d.go", "b.go", "c.go"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			indexer := New(nil, nil, Config{})
			graph := entities.NewGraph(nil, nil, nil)
			decls := twinDecls(testCase.files)

			var first []entities.Decl

			for run := range 64 {
				cut, err := indexer.cutWiring(context.Background(), graph, decls, newLocator())
				if err != nil {
					t.Fatalf("cutWiring() = %v", err)
				}

				if run == 0 {
					first = cut.decls
				}

				if !reflect.DeepEqual(cut.decls, first) {
					t.Fatalf("run %d: decls = %v, want the first run's %v", run, cut.decls, first)
				}
			}

			if !slices.IsSortedFunc(first, compareDeclKeys) {
				t.Fatalf("cutWiring() = %v, want key order", first)
			}
		})
	}
}

type (
	selectingSyntax struct {
		cuttingSyntax

		globs   []string
		refused []string
	}

	selection struct {
		globs     []string
		kind      entities.DeclKind
		receiver  string
		generated []string
		testSide  []string
		refused   []string
	}
)

func (syntax selectingSyntax) Functions() []string { return syntax.globs }

func (syntax selectingSyntax) Slice(
	ctx context.Context,
	fn entities.Decl,
	source []byte,
) (Sliced, bool, error) {
	if slices.Contains(syntax.refused, fn.Key.Name) {
		return Sliced{}, false, nil
	}

	return syntax.cuttingSyntax.Slice(ctx, fn, source)
}

func selectedCut(t *testing.T, pick selection) (wiringCut, error) {
	t.Helper()

	ctrl := gomock.NewController(t)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), "a.go").Return([]byte(sliceSource), nil).AnyTimes()

	decls := make([]entities.Decl, 0, 3)

	for _, name := range []string{"Run", "One", "Two"} {
		decls = append(decls, entities.Decl{
			Key:       sliceKey(pick.kind, pick.receiver+name),
			Span:      entities.Span{Start: 11, End: 39},
			Generated: slices.Contains(pick.generated, name),
			IsTest:    slices.Contains(pick.testSide, name),
		})
	}

	head := entities.Decl{Key: sliceKey(entities.KindFunc, "Head")}
	syntax := selectingSyntax{
		cuttingSyntax: cuttingSyntax{MockSyntax: NewMockSyntax(ctrl), sliced: Sliced{Head: head}},
		globs:         pick.globs,
		refused:       pick.refused,
	}
	indexer := New(nil, tree, Config{Languages: []Language{{
		Name: "go", Extensions: []string{".go"}, Syntax: syntax,
	}}})

	return indexer.cutWiring(
		context.Background(), entities.NewGraph(decls, nil, nil), decls, newLocator(),
	)
}

func TestCutWiringCutsTheFunctionsTheSelectorNames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		pick    selection
		wantCut []string
	}{
		{
			name: "no section cuts nothing",
			pick: selection{kind: entities.KindFunc},
		},
		{
			name:    "a name cuts the function it spells",
			pick:    selection{globs: []string{"Run"}, kind: entities.KindFunc},
			wantCut: []string{"Run"},
		},
		{
			name:    "a glob cuts every function it matches",
			pick:    selection{globs: []string{"O*", "Tw?"}, kind: entities.KindFunc},
			wantCut: []string{"One", "Two"},
		},
		{
			name: "a glob alive on one match survives the refusal of another",
			pick: selection{
				globs: []string{"*"}, kind: entities.KindFunc, refused: []string{"One"},
			},
			wantCut: []string{"Run", "Two"},
		},
		{
			name: "a method is named with its receiver",
			pick: selection{
				globs: []string{"*.Run"}, kind: entities.KindMethod, receiver: "Server.",
			},
			wantCut: []string{"Server.Run"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cut, err := selectedCut(t, testCase.pick)
			if err != nil {
				t.Fatalf("cutWiring() = %v", err)
			}

			var got []string

			for key := range cut.replaced {
				got = append(got, key.Name)
			}

			slices.Sort(got)

			if !slices.Equal(got, testCase.wantCut) {
				t.Fatalf("replaced = %v, want %v", got, testCase.wantCut)
			}
		})
	}
}

func TestCutFileNeverCutsAGeneratedOrTestSideRoot(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		generated bool
		testSide  bool
		wantCut   bool
	}{
		{name: "a plain root is cut", wantCut: true},
		{name: "a generated root is left whole", generated: true},
		{name: "a test-side root is left whole", testSide: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			tree := NewMockTree(ctrl)
			tree.EXPECT().Read(gomock.Any(), "a.go").Return([]byte(sliceSource), nil).AnyTimes()

			run := entities.Decl{
				Key:       sliceKey(entities.KindFunc, "Run"),
				Span:      entities.Span{Start: 11, End: 39},
				Generated: testCase.generated,
				IsTest:    testCase.testSide,
			}
			syntax := cuttingSyntax{
				MockSyntax: NewMockSyntax(ctrl),
				sliced:     Sliced{Head: entities.Decl{Key: run.Key}},
			}
			indexer := New(nil, tree, Config{Languages: []Language{{
				Name: "go", Extensions: []string{".go"}, Syntax: syntax,
			}}})
			cut := wiringCut{
				replaced: map[entities.DeclKey]bool{},
				added:    map[entities.DeclKey]bool{},
			}
			byFile := map[string][]entities.Decl{"a.go": {run}}

			err := indexer.cutFile(
				context.Background(), "a.go", byFile, map[entities.DeclKey]bool{run.Key: true},
				&cut, newLocator(),
			)
			if err != nil {
				t.Fatalf("cutFile() = %v", err)
			}

			if cut.replaced[run.Key] != testCase.wantCut {
				t.Fatalf("replaced = %v, want cut = %v", cut.replaced, testCase.wantCut)
			}
		})
	}
}

func TestCutWiringKeepsAGlobToTheFilesItsLanguageOwns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		goGlobs []string
		xGlobs  []string
		want    []string
	}{
		{
			name:    "each glob cuts in its own files",
			goGlobs: []string{"A"},
			xGlobs:  []string{"B"},
			want:    []string{"a.go:A", "b.x:B"},
		},
		{
			name:    "a name two files share is cut only where its language owns it",
			goGlobs: []string{"A"},
			xGlobs:  []string{"A", "B"},
			want:    []string{"a.go:A", "b.x:A", "b.x:B"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			tree := NewMockTree(ctrl)
			tree.EXPECT().
				Read(gomock.Any(), gomock.Any()).
				Return([]byte(sliceSource), nil).
				AnyTimes()

			cutting := cuttingSyntax{MockSyntax: NewMockSyntax(ctrl)}
			goSyntax := selectingSyntax{cuttingSyntax: cutting, globs: testCase.goGlobs}
			xSyntax := selectingSyntax{cuttingSyntax: cutting, globs: testCase.xGlobs}
			indexer := New(nil, tree, Config{Languages: []Language{
				{Name: "go", Extensions: []string{".go"}, Syntax: goSyntax},
				{Name: "x", Extensions: []string{".x"}, Syntax: xSyntax},
			}})

			decls := make([]entities.Decl, 0, 3)

			for _, name := range []string{"a.go:A", "b.x:A", "b.x:B"} {
				file, fn, _ := strings.Cut(name, ":")
				key := entities.DeclKey{File: file, Kind: entities.KindFunc, Name: fn}
				span := entities.Span{Start: 11, End: 39}
				decls = append(decls, entities.Decl{Key: key, Span: span})
			}

			graph := entities.NewGraph(decls, nil, nil)

			cut, err := indexer.cutWiring(context.Background(), graph, decls, newLocator())
			if err != nil {
				t.Fatalf("cutWiring() = %v", err)
			}

			var got []string

			for key := range cut.replaced {
				got = append(got, key.File+":"+key.Name)
			}

			slices.Sort(got)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("replaced = %v, want %v", got, testCase.want)
			}
		})
	}
}
