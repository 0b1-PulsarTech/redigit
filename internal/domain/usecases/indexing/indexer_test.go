package indexing

import (
	"context"
	"errors"
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

func TestIndexerAppliesRuleEdgesAndAnchors(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := goDecl("a_mock.go", "MockRepo", 0, 20)
	port := goDecl("a.go", "Repo", 11, 24)
	suite := goDecl("a_test.go", "TestRepo", 0, 20)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil).AnyTimes()

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, file string, _ []byte) ([]entities.Decl, error) {
			switch file {
			case "a.go":
				return []entities.Decl{port}, nil
			case "a_mock.go":
				return []entities.Decl{mock}, nil
			}

			return []entities.Decl{suite}, nil
		}).
		AnyTimes()

	rules := NewMockRulePack(ctrl)
	rules.EXPECT().Edges(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]entities.Edge{{Dependent: mock.Key, Provider: port.Key, Reason: entities.ReasonMirrors}},
		nil,
	)
	rules.EXPECT().Anchors(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]entities.Anchor{{Decl: suite.Key, To: port.Key, Reason: entities.ReasonTestOf}},
		nil,
	)

	result, err := New(quietResolver(ctrl), tree, Config{Languages: []Language{{
		Name:       "go",
		Extensions: []string{".go"},
		Syntax:     syntax,
		Rules:      rules,
	}}}).Index(
		context.Background(),
		entities.Snapshot{Files: []string{"a.go", "a_mock.go", "a_test.go"}},
	)
	if err != nil {
		t.Fatalf("Index() = %v", err)
	}

	// A mock satisfies its interface structurally, so nothing a resolver can
	// see connects the two. If the rule pack's edge did not survive, the mock
	// is free to be born before the port it mirrors.
	needs := result.Graph.Needs(mock.Key)
	if len(needs) != 1 || needs[0].Reason != entities.ReasonMirrors {
		t.Fatalf("mock needs %v, want one mirrors edge", needs)
	}

	if anchors := result.Graph.Anchors(); len(anchors) != 1 ||
		anchors[0].Reason != entities.ReasonTestOf {
		t.Fatalf("anchors = %v, want one test-of anchor", anchors)
	}
}

// atomRules is a rule pack that also groups declarations.
type atomRules struct {
	*MockRulePack

	atoms map[entities.DeclKey]string
}

func (r atomRules) Atoms(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) (map[entities.DeclKey]string, error) {
	return r.atoms, nil
}

func TestIndexerGroupsDeclarationsTheRulesName(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	typ := goDecl("a.go", "Err", 0, 10)
	method := goDecl("b.go", "Err.Error", 0, 10)
	generated := goDecl("c.go", "Impl", 0, 10)
	generated.Atom = "c.go"
	loose := goDecl("d.go", "Loose", 0, 10)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil).AnyTimes()

	byFile := map[string]entities.Decl{
		"a.go": typ, "b.go": method, "c.go": generated, "d.go": loose,
	}

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, file string, _ []byte) ([]entities.Decl, error) {
			return []entities.Decl{byFile[file]}, nil
		}).
		AnyTimes()

	rules := NewMockRulePack(ctrl)
	rules.EXPECT().Edges(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
	rules.EXPECT().Anchors(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	grouping := atomRules{MockRulePack: rules, atoms: map[entities.DeclKey]string{
		typ.Key: "set", method.Key: "set", generated.Key: "set",
	}}

	result, err := New(quietResolver(ctrl), tree, Config{Languages: []Language{{
		Name:       "go",
		Extensions: []string{".go"},
		Syntax:     syntax,
		Rules:      grouping,
	}}}).Index(
		context.Background(),
		entities.Snapshot{Files: []string{"a.go", "b.go", "c.go", "d.go"}},
	)
	if err != nil {
		t.Fatalf("Index() = %v", err)
	}

	got := map[entities.DeclKey]string{}
	for _, decl := range result.Graph.Decls() {
		got[decl.Key] = decl.Atom
	}

	want := map[entities.DeclKey]string{
		typ.Key: "set", method.Key: "set", generated.Key: "c.go", loose.Key: "",
	}

	for key, atom := range want {
		if got[key] != atom {
			t.Fatalf("atom of %s = %q, want %q", key, got[key], atom)
		}
	}
}
