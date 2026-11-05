package indexing

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

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
	mate := goDecl("e.go", "Mate", 0, 10)
	mate.Atom = "c.go"

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil).AnyTimes()

	byFile := map[string]entities.Decl{
		"a.go": typ, "b.go": method, "c.go": generated, "d.go": loose, "e.go": mate,
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
		entities.Snapshot{Files: []string{"a.go", "b.go", "c.go", "d.go", "e.go"}},
	)
	if err != nil {
		t.Fatalf("Index() = %v", err)
	}

	got := map[entities.DeclKey]string{}
	for _, decl := range result.Graph.Decls() {
		got[decl.Key] = decl.Atom
	}

	want := map[entities.DeclKey]string{
		typ.Key: "set", method.Key: "set", generated.Key: "set", mate.Key: "set", loose.Key: "",
	}

	for key, atom := range want {
		if got[key] != atom {
			t.Fatalf("atom of %s = %q, want %q", key, got[key], atom)
		}
	}
}

// filterRules is a rule pack that also removes the edges it calls noise.
type filterRules struct {
	*MockRulePack

	drop entities.EdgeReason
}

func (r filterRules) FilterEdges(_ entities.Snapshot, edges []entities.Edge) []entities.Edge {
	kept := edges[:0:0]

	for _, edge := range edges {
		if edge.Reason != r.drop {
			kept = append(kept, edge)
		}
	}

	return kept
}

func TestIndexerLetsTheRulesDropMergedEdges(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := goDecl("a_mock.go", "MockRepo", 0, 20)
	port := goDecl("a.go", "Repo", 11, 24)

	tree := NewMockTree(ctrl)
	tree.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte(goSource), nil).AnyTimes()

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, file string, _ []byte) ([]entities.Decl, error) {
			if file == "a.go" {
				return []entities.Decl{port}, nil
			}

			return []entities.Decl{mock}, nil
		}).
		AnyTimes()

	rules := NewMockRulePack(ctrl)
	rules.EXPECT().Edges(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]entities.Edge{
			{Dependent: mock.Key, Provider: port.Key, Reason: entities.ReasonMirrors},
			{Dependent: mock.Key, Provider: port.Key, Reason: entities.ReasonImplements},
		},
		nil,
	)
	rules.EXPECT().Anchors(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	result, err := New(quietResolver(ctrl), tree, Config{Languages: []Language{{
		Name:       "go",
		Extensions: []string{".go"},
		Syntax:     syntax,
		Rules:      filterRules{MockRulePack: rules, drop: entities.ReasonImplements},
	}}}).Index(
		context.Background(),
		entities.Snapshot{Files: []string{"a.go", "a_mock.go"}},
	)
	if err != nil {
		t.Fatalf("Index() = %v", err)
	}

	needs := result.Graph.Needs(mock.Key)
	if len(needs) != 1 || needs[0].Reason != entities.ReasonMirrors {
		t.Fatalf("mock needs %v, want only the mirrors edge", needs)
	}
}
