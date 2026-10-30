package shaping

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	fakePass struct {
		name string
		run  func(ctx context.Context, draft *Draft) (Moves, error)
		ran  *[]string
	}

	pipelineCase struct {
		name        string
		passes      func(ran *[]string) []Pass
		cancelled   bool
		unobserved  bool
		expect      func(observer *MockObserver)
		wantErr     error
		wantErrText string
		wantRan     []string
		wantMoves   int
		wantSlots   int
	}
)

func (pass fakePass) Name() string { return pass.name }

func (pass fakePass) Apply(ctx context.Context, draft *Draft) (Moves, error) {
	*pass.ran = append(*pass.ran, pass.name)

	return pass.run(ctx, draft)
}

func spawnAfterFirst(name string) func(context.Context, *Draft) (Moves, error) {
	return func(_ context.Context, draft *Draft) (Moves, error) {
		commit := entities.PlannedCommit{Subject: "new"}
		moves, _ := draft.Spawn(0, commit, []entities.DeclKey{key(name)})

		return moves, nil
	}
}

func breakProvider(_ context.Context, draft *Draft) (Moves, error) {
	moves, _ := draft.Merge(0, 2)

	return moves, nil
}

func fail(err error) func(context.Context, *Draft) (Moves, error) {
	return func(context.Context, *Draft) (Moves, error) { return nil, err }
}

func pipelineCases(boom error) []pipelineCase {
	broken := entities.Edge{
		Dependent: key("a"), Provider: key("b"), Reason: entities.ReasonReference,
	}

	return []pipelineCase{
		{
			name:      "no passes",
			passes:    func(*[]string) []Pass { return nil },
			expect:    func(*MockObserver) {},
			wantSlots: 3,
		},
		{
			name: "passes run in order and every move is observed",
			passes: func(ran *[]string) []Pass {
				return []Pass{
					fakePass{"first", spawnAfterFirst("z"), ran},
					fakePass{"second", fail(nil), ran},
				}
			},
			expect: func(observer *MockObserver) {
				observer.EXPECT().Moved("first", Move{Decl: key("z"), From: 2, To: 3})
			},
			wantRan:   []string{"first", "second"},
			wantMoves: 1,
			wantSlots: 4,
		},
		{
			name: "a pass adding a violation is refused and restored",
			passes: func(ran *[]string) []Pass {
				return []Pass{
					fakePass{"breaker", breakProvider, ran},
					fakePass{"after", spawnAfterFirst("z"), ran},
				}
			},
			expect: func(observer *MockObserver) {
				gomock.InOrder(
					observer.EXPECT().Refused("breaker", []entities.Edge{broken}),
					observer.EXPECT().Moved("after", gomock.Any()),
				)
			},
			wantRan:   []string{"breaker", "after"},
			wantMoves: 1,
			wantSlots: 4,
		},
		{
			name: "a pass error names the pass and its index",
			passes: func(ran *[]string) []Pass {
				return []Pass{
					fakePass{"ok", fail(nil), ran},
					fakePass{"bad", fail(boom), ran},
					fakePass{"never", fail(nil), ran},
				}
			},
			expect:      func(*MockObserver) {},
			wantErr:     boom,
			wantErrText: "pass 1 (bad): boom",
			wantRan:     []string{"ok", "bad"},
			wantSlots:   3,
		},
		{
			name: "a cancelled context stops before the next pass",
			passes: func(ran *[]string) []Pass {
				return []Pass{fakePass{"first", fail(nil), ran}}
			},
			cancelled:   true,
			expect:      func(*MockObserver) {},
			wantErr:     context.Canceled,
			wantErrText: "pass 0 (first): context canceled",
			wantSlots:   3,
		},
	}
}

func silentCases() []pipelineCase {
	return []pipelineCase{{
		name: "a nil observer still shapes and refuses without panicking",
		passes: func(ran *[]string) []Pass {
			return []Pass{
				fakePass{"breaker", breakProvider, ran},
				fakePass{"after", spawnAfterFirst("z"), ran},
			}
		},
		unobserved: true,
		expect:     func(*MockObserver) {},
		wantRan:    []string{"breaker", "after"},
		wantMoves:  1,
		wantSlots:  4,
	}}
}

func TestPipelineApply(t *testing.T) {
	t.Parallel()

	rows := [][]string{{"b"}, {"a"}, {"z"}}
	specs := []edgeSpec{{"a", "b", entities.ReasonReference}}

	for _, testCase := range slices.Concat(pipelineCases(errors.New("boom")), silentCases()) {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mock := NewMockObserver(gomock.NewController(t))
			testCase.expect(mock)

			var (
				ran      []string
				observer Observer = mock
			)

			if testCase.unobserved {
				observer = nil
			}

			draft := buildDraft(rows, specs)
			pipeline := NewPipeline(observer, testCase.passes(&ran)...)

			ctx, cancel := context.WithCancel(t.Context())
			if testCase.cancelled {
				cancel()
			}

			defer cancel()

			moves, err := pipeline.Apply(ctx, draft)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Apply() error = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && err.Error() != testCase.wantErrText {
				t.Fatalf("Apply() error text = %q, want %q", err.Error(), testCase.wantErrText)
			}

			if !slices.Equal(ran, testCase.wantRan) {
				t.Fatalf("ran = %v, want %v", ran, testCase.wantRan)
			}

			if len(moves) != testCase.wantMoves || len(draft.Slots()) != testCase.wantSlots {
				t.Fatalf(
					"moves = %d, slots = %d, want %d, %d",
					len(moves), len(draft.Slots()), testCase.wantMoves, testCase.wantSlots,
				)
			}

			if len(draft.Violations()) != 0 {
				t.Fatalf("Apply() left violations: %v", draft.Violations())
			}
		})
	}
}

func TestPipelineRefusesAnAtomSplit(t *testing.T) {
	t.Parallel()

	atomEdge := func(dependent, provider string) entities.Edge {
		return entities.Edge{
			Dependent: key(dependent), Provider: key(provider), Reason: ReasonAtomSplit,
		}
	}

	testCases := []struct {
		name string
		rows [][]string
		move string
		to   int
		want []entities.Edge
	}{
		{
			"a member leaves its slot",
			[][]string{{"q@t", "g@t"}, {"a"}},
			"g", 1,
			[]entities.Edge{atomEdge("q", "g")},
		},
		{
			"a further split of an atom already apart",
			[][]string{{"a@t"}, {"b@t", "c@t"}, {"x"}},
			"c", 2,
			[]entities.Edge{atomEdge("b", "a"), atomEdge("c", "a")},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			splitter := func(_ context.Context, draft *Draft) (Moves, error) {
				return Moves{draft.relocate(key(testCase.move), testCase.to)}, nil
			}

			observer := NewMockObserver(gomock.NewController(t))
			observer.EXPECT().Refused("splitter", testCase.want)

			var ran []string

			draft := atomDraft(testCase.rows)
			before := draft.AtomSplits()
			pipeline := NewPipeline(observer, fakePass{"splitter", splitter, &ran})

			moves, err := pipeline.Apply(t.Context(), draft)
			if err != nil || len(moves) != 0 || !slices.Equal(draft.AtomSplits(), before) {
				t.Fatalf("Apply() = %d moves, %v, splits %v", len(moves), err, draft.AtomSplits())
			}
		})
	}
}

func TestPipelineName(t *testing.T) {
	t.Parallel()

	if got := NewPipeline(nil).Name(); got != "pipeline" {
		t.Fatalf("Name() = %q, want pipeline", got)
	}
}
