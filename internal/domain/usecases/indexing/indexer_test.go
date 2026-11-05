package indexing

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const goSource = "package a\n\nfunc One() {}\n"

func goDecl(file, name string, start, end int) entities.Decl {
	return entities.Decl{
		Key:      entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit:     "a",
		Span:     entities.Span{Start: start, End: end},
		Selector: entities.Location{File: file, Line: 2, Char: 5},
	}
}

// quietResolver answers every query with nothing, which is the right default
// for a test about reading rather than about edges.
func quietResolver(ctrl *gomock.Controller) *MockResolver {
	resolver := NewMockResolver(ctrl)
	resolver.EXPECT().References(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	resolver.EXPECT().Implementations(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	return resolver
}

func TestIndexerRefusesAnEmptyRun(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil).AnyTimes()

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	goLang := Language{Name: "go", Extensions: []string{".go"}, Syntax: syntax, Resolved: true}

	testCases := []struct {
		name  string
		langs []Language
		files []string
	}{
		{"no language configured", nil, []string{"a.go"}},
		{
			"no declaration found is a hole, not an empty success",
			[]Language{goLang},
			[]string{"a.go"},
		},
		{"no file the languages own", []Language{goLang}, []string{"README.md"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(quietResolver(ctrl), tree, Config{Languages: testCase.langs}).
				Index(context.Background(), entities.Snapshot{Files: testCase.files})
			if !errors.Is(err, apperr.ErrIncomplete) {
				t.Fatalf("Index() = %v, want %v", err, apperr.ErrIncomplete)
			}
		})
	}
}

func TestLanguageOwns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		lang Language
		file string
		want bool
	}{
		{"an owned extension", Language{Extensions: []string{".go"}}, "a.go", true},
		{"an unowned extension", Language{Extensions: []string{".go"}}, "a.ts", false},
		{"no extensions owns nothing", Language{}, "a.go", false},
		{
			"AnyExtension claims a known extension",
			Language{Extensions: []string{AnyExtension}},
			"a.go",
			true,
		},
		{
			"AnyExtension claims an unknown extension",
			Language{Extensions: []string{AnyExtension}},
			"a.ts",
			true,
		},
		{
			"AnyExtension claims a file with none",
			Language{Extensions: []string{AnyExtension}},
			"Dockerfile",
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.lang.Owns(testCase.file); got != testCase.want {
				t.Fatalf("Owns(%q) = %v, want %v", testCase.file, got, testCase.want)
			}
		})
	}
}

type failingParts struct {
	tree     Tree
	syntax   Syntax
	resolver Resolver
	rules    RulePack
}

func onlyOneDecl(ctrl *gomock.Controller) (*MockTree, *MockSyntax) {
	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil)

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]entities.Decl{goDecl("a.go", "One", 11, 24)}, nil)

	return tree, syntax
}

func unreadableTree(ctrl *gomock.Controller, boom error) failingParts {
	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return(nil, boom)

	return failingParts{tree: tree, syntax: NewMockSyntax(ctrl), resolver: quietResolver(ctrl)}
}

func unparsableFile(ctrl *gomock.Controller, boom error) failingParts {
	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil)

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().Declarations(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, boom)

	return failingParts{tree: tree, syntax: syntax, resolver: quietResolver(ctrl)}
}

func failingRules(ctrl *gomock.Controller, boom error) failingParts {
	tree, syntax := onlyOneDecl(ctrl)

	rules := NewMockRulePack(ctrl)
	rules.EXPECT().Edges(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, boom)

	return failingParts{tree: tree, syntax: syntax, resolver: quietResolver(ctrl), rules: rules}
}

func TestIndexerPropagatesFailures(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	testCases := []struct {
		name  string
		setup func(*gomock.Controller, error) failingParts
	}{
		{"a file that cannot be read stops the run", unreadableTree},
		{"a file that cannot be parsed stops the run", unparsableFile},
		{"a rule pack that fails stops the run", failingRules},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			parts := testCase.setup(ctrl, errBoom)

			_, err := New(parts.resolver, parts.tree, Config{Languages: []Language{{
				Name:       "go",
				Extensions: []string{".go"},
				Syntax:     parts.syntax,
				Rules:      parts.rules,
			}}}).Index(context.Background(), entities.Snapshot{Files: []string{"a.go"}})

			if !errors.Is(err, errBoom) {
				t.Fatalf("Index() = %v, want it to carry %v", err, errBoom)
			}
		})
	}
}

type orderFixture struct {
	files    []string
	sources  map[string][]byte
	decls    map[string][]entities.Decl
	all      []entities.Decl
	failures map[entities.Location]bool
}

func newOrderFixture(fileCount, perFile, failing int) orderFixture {
	fixture := orderFixture{
		sources:  map[string][]byte{},
		decls:    map[string][]entities.Decl{},
		failures: map[entities.Location]bool{},
	}

	for file := range fileCount {
		path := fmt.Sprintf("f%d.go", file)
		source := "package p\n\n"

		for index := range perFile {
			name := fmt.Sprintf("F%d_%d", file, index)
			text := fmt.Sprintf("func %s() {}", name)
			decl := entities.Decl{
				Key:      entities.DeclKey{File: path, Kind: entities.KindFunc, Name: name},
				Unit:     "p",
				Span:     entities.Span{Start: len(source), End: len(source) + len(text)},
				Selector: entities.Location{File: path, Line: 2 + 2*index, Char: 5},
			}
			decl.IsInterface = len(fixture.all) < failing

			if decl.IsInterface {
				fixture.failures[decl.Selector] = true
			}

			source += text + "\n\n"

			fixture.decls[path] = append(fixture.decls[path], decl)
			fixture.all = append(fixture.all, decl)
		}

		fixture.files = append(fixture.files, path)
		fixture.sources[path] = []byte(source)
	}

	return fixture
}

func (fixture orderFixture) index(t *testing.T, reverse bool) (Result, error) {
	t.Helper()

	indexer, snap := fixture.build(t, reverse)

	return indexer.Index(context.Background(), snap)
}

func (fixture orderFixture) build(t *testing.T, reverse bool) (Indexer, entities.Snapshot) {
	t.Helper()

	ctrl := gomock.NewController(t)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, file string) ([]byte, error) { return fixture.sources[file], nil },
	).AnyTimes()

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().Declarations(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, file string, _ []byte) ([]entities.Decl, error) {
			return slices.Clone(fixture.decls[file]), nil
		},
	).AnyTimes()

	resolver := NewMockResolver(ctrl)
	resolver.EXPECT().References(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, at entities.Location) ([]entities.Location, error) {
			if fixture.failures[at] {
				return nil, errors.New("references refused")
			}

			return fixture.readers(at, reverse), nil
		},
	).AnyTimes()
	resolver.EXPECT().Implementations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, at entities.Location) ([]entities.Location, error) {
			return nil, errors.New("implementations refused")
		},
	).AnyTimes()

	files := slices.Clone(fixture.files)
	if reverse {
		slices.Reverse(files)
	}

	return New(resolver, tree, Config{
		Concurrency: 8,
		Languages: []Language{{
			Name: "go", Extensions: []string{".go"}, Syntax: syntax, Resolved: true,
		}},
	}), entities.Snapshot{Files: files}
}

func (fixture orderFixture) readers(at entities.Location, reverse bool) []entities.Location {
	position := slices.IndexFunc(fixture.all, func(decl entities.Decl) bool {
		return decl.Selector == at
	})

	var out []entities.Location

	for step := 1; step <= 3; step++ {
		reader := fixture.all[(position+step)%len(fixture.all)]
		at := entities.Location{File: reader.Key.File, Line: reader.Selector.Line, Char: 1}
		out = append(out, at)
	}

	if reverse {
		slices.Reverse(out)
	}

	return out
}

func TestIndexIgnoresInputOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		fixture orderFixture
	}{
		{"edges from several files", newOrderFixture(4, 5, 0)},
		{"declarations that tie on a blind spot", newOrderFixture(8, 25, 8)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			forward, err := testCase.fixture.index(t, false)
			if err != nil {
				t.Fatalf("Index() = %v", err)
			}

			backward, err := testCase.fixture.index(t, true)
			if err != nil {
				t.Fatalf("Index() reversed = %v", err)
			}

			got := []any{backward.Graph.Decls(), backward.Graph.Edges(), backward.Blind}
			want := []any{forward.Graph.Decls(), forward.Graph.Edges(), forward.Blind}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("reversed inputs gave a different result:\n%v\nwant:\n%v", got, want)
			}
		})
	}
}
