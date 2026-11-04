package idiom

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

var errBoom = errors.New("boom")

type (
	stubGate struct{}

	gatesCase struct {
		name        string
		gaters      int
		isCancelled bool
		wantNames   []string
		wantErr     error
	}

	edgeCase struct {
		name      string
		setup     func(ctrl *gomock.Controller) (Provider, []rules.Rule)
		want      []entities.Edge
		wantError error
		wantText  string
	}
)

func (stubGate) Name() string { return "stub" }

func (stubGate) Measure(entities.Graph, entities.Ordering) entities.Measure {
	return entities.Measure{}
}

func goDecl(name string) entities.Decl {
	return entities.Decl{Key: entities.DeclKey{File: "a/a.go", Kind: entities.KindFunc, Name: name}}
}

func dartDecl(name string) entities.Decl {
	return entities.Decl{
		Key: entities.DeclKey{File: "lib/a.dart", Kind: entities.KindFunc, Name: name},
	}
}

func edgeBetween(from, to entities.Decl) entities.Edge {
	return entities.Edge{Dependent: from.Key, Provider: to.Key, Reason: entities.ReasonProvides}
}

func TestPackEdges(t *testing.T) {
	t.Parallel()

	one, two := goDecl("One"), goDecl("Two")
	scoped := []entities.Decl{one, two}
	testCases := []edgeCase{
		{
			name: "edge rules get the Go declarations and the facts of them",
			setup: func(ctrl *gomock.Controller) (Provider, []rules.Rule) {
				facts := NewMockFacts(ctrl)
				provider := NewMockProvider(ctrl)
				provider.EXPECT().Facts(gomock.Any(), gomock.Any(), scoped).Return(facts, nil)

				first, second := NewMockEdgeRule(ctrl), NewMockEdgeRule(ctrl)
				first.EXPECT().
					Edges(gomock.Any(), gomock.Any(), scoped, facts).
					Return([]entities.Edge{edgeBetween(one, two)}, nil)
				second.EXPECT().
					Edges(gomock.Any(), gomock.Any(), scoped, facts).
					Return([]entities.Edge{edgeBetween(two, one)}, nil)

				return provider, []rules.Rule{first, second}
			},
			want: []entities.Edge{edgeBetween(one, two), edgeBetween(two, one)},
		},
		{
			name: "a rule without the edge capability is not asked",
			setup: func(ctrl *gomock.Controller) (Provider, []rules.Rule) {
				provider := NewMockProvider(ctrl)

				return provider, []rules.Rule{NewMockAnchorRule(ctrl), NewMockAtomRule(ctrl)}
			},
		},
		{
			name: "the failing rule is named by its index and type",
			setup: func(ctrl *gomock.Controller) (Provider, []rules.Rule) {
				provider := NewMockProvider(ctrl)
				provider.EXPECT().Facts(gomock.Any(), gomock.Any(), scoped).
					Return(NewMockFacts(ctrl), nil)

				fine, broken := NewMockEdgeRule(ctrl), NewMockEdgeRule(ctrl)
				fine.EXPECT().Edges(gomock.Any(), gomock.Any(), scoped, gomock.Any()).
					Return(nil, nil)
				broken.EXPECT().Edges(gomock.Any(), gomock.Any(), scoped, gomock.Any()).
					Return(nil, errBoom)
				broken.EXPECT().Type().Return("go-router").AnyTimes()

				return provider, []rules.Rule{fine, broken}
			},
			wantError: errBoom,
			wantText:  "rules[1] (go-router)",
		},
		{
			name: "a facts failure is wrapped",
			setup: func(ctrl *gomock.Controller) (Provider, []rules.Rule) {
				provider := NewMockProvider(ctrl)
				provider.EXPECT().Facts(gomock.Any(), gomock.Any(), scoped).Return(nil, errBoom)

				return provider, []rules.Rule{NewMockEdgeRule(ctrl)}
			},
			wantError: errBoom,
			wantText:  "idiom facts",
		},
	}

	requirePackEdges(t, testCases)
}

func requirePackEdges(t *testing.T, testCases []edgeCase) {
	t.Helper()

	all := []entities.Decl{goDecl("One"), dartDecl("Other"), goDecl("Two")}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider, built := testCase.setup(gomock.NewController(t))
			pack := NewPack(provider, []string{".go"}, built...)

			got, err := pack.Edges(context.Background(), entities.Snapshot{}, all)
			if !errors.Is(err, testCase.wantError) {
				t.Fatalf("Edges() err = %v, want %v", err, testCase.wantError)
			}

			if err != nil && !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("Edges() err = %q, want it to hold %q", err, testCase.wantText)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("Edges() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestPackAnchorsAndAtoms(t *testing.T) {
	t.Parallel()

	one, two := goDecl("One"), goDecl("Two")
	decls := []entities.Decl{one, dartDecl("Other"), two}
	anchor := entities.Anchor{Decl: one.Key, To: two.Key, Reason: entities.ReasonProvides}
	anchorsOf := func(pack Pack) (any, error) {
		return pack.Anchors(context.Background(), entities.Snapshot{}, decls)
	}
	atomsOf := func(pack Pack) (any, error) {
		return pack.Atoms(context.Background(), entities.Snapshot{}, decls)
	}

	testCases := []struct {
		name      string
		call      func(pack Pack) (any, error)
		anchorErr error
		atomErr   error
		want      any
		wantText  string
	}{
		{
			name: "the anchors of the Go declarations",
			call: anchorsOf,
			want: []entities.Anchor{anchor},
		},
		{
			name: "the atoms of the Go declarations",
			call: atomsOf,
			want: map[entities.DeclKey]string{one.Key: "di"},
		},
		{
			name:      "an anchor rule failure names its rule",
			call:      anchorsOf,
			anchorErr: errBoom,
			wantText:  "rules[0] (go-di)",
		},
		{
			name:     "an atom rule failure names its rule",
			call:     atomsOf,
			atomErr:  errBoom,
			wantText: "rules[1] (go-di)",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			known := NewMockFacts(ctrl)
			provider := NewMockProvider(ctrl)
			provider.EXPECT().Facts(gomock.Any(), gomock.Any(), []entities.Decl{one, two}).
				Return(known, nil).AnyTimes()

			anchors, atoms := NewMockAnchorRule(ctrl), NewMockAtomRule(ctrl)
			anchors.EXPECT().Anchors(gomock.Any(), gomock.Any(), []entities.Decl{one, two}, known).
				Return([]entities.Anchor{anchor}, testCase.anchorErr).AnyTimes()
			atoms.EXPECT().Atoms(gomock.Any(), gomock.Any(), []entities.Decl{one, two}, known).
				Return(map[entities.DeclKey]string{one.Key: "di"}, testCase.atomErr).AnyTimes()
			anchors.EXPECT().Type().Return("go-di").AnyTimes()
			atoms.EXPECT().Type().Return("go-di").AnyTimes()

			pack := NewPack(provider, []string{".go"}, anchors, atoms, NewMockEdgeRule(ctrl))

			got, err := testCase.call(pack)
			if testCase.wantText != "" {
				if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), testCase.wantText) {
					t.Fatalf("err = %v, want %q wrapping boom", err, testCase.wantText)
				}

				return
			}

			if err != nil || !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("got %v, %v, want %v", got, err, testCase.want)
			}
		})
	}
}

type namedGate struct{ stubGate }

func (namedGate) Measure(entities.Graph, entities.Ordering) entities.Measure {
	return entities.Measure{Name: "measure"}
}

func measureNames(gates []auditing.Gate) []string {
	names := make([]string, 0, len(gates))

	for _, gate := range gates {
		names = append(names, gate.Measure(entities.Graph{}, entities.Ordering{}).Name)
	}

	return names
}

func TestPackGates(t *testing.T) {
	t.Parallel()

	testCases := []gatesCase{
		{name: "one gate keeps its measure name", gaters: 1, wantNames: []string{"measure"}},
		{
			name:      "several gates name their rule",
			gaters:    2,
			wantNames: []string{"rules[0] (go-di): measure", "rules[1] (go-di): measure"},
		},
		{
			name:        "a gate outliving its context reports it",
			gaters:      1,
			isCancelled: true,
			wantNames:   []string{"measure"},
			wantErr:     context.Canceled,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			one := goDecl("One")
			decls := []entities.Decl{one, dartDecl("Other")}
			ctrl := gomock.NewController(t)
			facts := NewMockFacts(ctrl)
			provider := NewMockProvider(ctrl)

			if !testCase.isCancelled {
				provider.EXPECT().Facts(gomock.Any(), gomock.Any(), []entities.Decl{one}).
					Return(facts, nil).Times(testCase.gaters)
			}

			var seen []error

			gaters := make([]rules.Rule, 0, testCase.gaters)

			for range testCase.gaters {
				gater := NewMockGater(ctrl)
				gater.EXPECT().Type().Return("go-di").AnyTimes()
				gater.EXPECT().Gate(gomock.Any()).DoAndReturn(
					func(factsOf FactsOf) auditing.Gate {
						got, err := factsOf(decls)
						if (got == nil) != (err != nil) || (err == nil && got != facts) {
							t.Errorf("factsOf() = %v, %v", got, err)
						}

						seen = append(seen, err)

						return namedGate{}
					},
				)

				gaters = append(gaters, gater)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if testCase.isCancelled {
				cancel()
			}

			gates := NewPack(provider, []string{".go"}, gaters...).Gates(ctx, entities.Snapshot{})
			names := measureNames(gates)

			if !slices.Equal(names, testCase.wantNames) {
				t.Fatalf("measure names = %v, want %v", names, testCase.wantNames)
			}

			for _, err := range seen {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("factsOf() err = %v, want %v", err, testCase.wantErr)
				}
			}
		})
	}
}
