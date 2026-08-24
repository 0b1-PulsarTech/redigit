package emitting

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func sampleGraph() entities.Graph {
	one := key("a.go", "One")
	two := key("a.go", "Two")

	return entities.NewGraph(
		[]entities.Decl{
			decl(one, entities.Span{Start: 0, End: 3}),
			decl(two, entities.Span{Start: 4, End: 7}),
		},
		nil,
		nil,
	)
}

func TestEmitterGrowsAFileAcrossCommitsAndReadsItOnce(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	// The final text is read once and reused: every commit's version of the
	// file is cut from it, never from an intermediate blob.
	source.EXPECT().
		Read(gomock.Any(), "a.go").
		Return([]byte("final"), FileMode("100644"), nil).
		Times(1)

	commits, err := New(source, growingRenderer(ctrl), countingSink(ctrl)).
		Emit(context.Background(), reviewedPlan(), sampleGraph())
	if err != nil {
		t.Fatalf("Emit() = %v", err)
	}

	testCases := []struct {
		name string
		want string
	}{
		{"the first commit holds only the first declaration", "One\n"},
		{"the second holds both, so the file grew rather than being born whole", "One\nTwo\n"},
	}

	for i, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := string(commits[i].Blobs[0].Handle)
			if got != testCase.want {
				t.Fatalf("commit %d wrote %q, want %q", i, got, testCase.want)
			}
		})
	}
}

func TestEmitterOrdersChunksAndCarriesSpanAndUnit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), "a.go").Return([]byte("final"), FileMode("100644"), nil)

	commits, err := New(source, growingRenderer(ctrl), countingSink(ctrl)).
		Emit(context.Background(), reviewedPlan(), sampleGraph())
	if err != nil {
		t.Fatalf("Emit() = %v", err)
	}

	chunk := commits[0].Chunks[0]
	if chunk.Key != key("a.go", "One") {
		t.Fatalf("chunk key = %v, want One", chunk.Key)
	}

	if chunk.Span != (entities.Span{Start: 0, End: 3}) {
		t.Fatalf("chunk span = %v, want {0 3}", chunk.Span)
	}

	if chunk.Unit != "a.go" {
		t.Fatalf("chunk unit = %q, want %q", chunk.Unit, "a.go")
	}
}

func TestEmitterReportsARiskOnlyForAForwardReference(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	one := key("a.go", "One")
	two := key("a.go", "Two")

	// One (born at position 0) needs Two, which is not born until position 1:
	// a forward reference the tool can see without compiling anything.
	graph := entities.NewGraph(
		[]entities.Decl{
			decl(one, entities.Span{Start: 0, End: 3}),
			decl(two, entities.Span{Start: 4, End: 7}),
		},
		[]entities.Edge{{Dependent: one, Provider: two, Reason: entities.ReasonReference}},
		nil,
	)

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), "a.go").Return([]byte("final"), FileMode("100644"), nil)

	commits, err := New(source, growingRenderer(ctrl), countingSink(ctrl)).
		Emit(context.Background(), reviewedPlan(), graph)
	if err != nil {
		t.Fatalf("Emit() = %v", err)
	}

	if got := len(commits[0].Risks); got != 1 {
		t.Fatalf("commit 0 risks = %d, want 1", got)
	}

	want := Risk{Chunk: one, Target: two, BornAt: 1}
	if commits[0].Risks[0] != want {
		t.Fatalf("commit 0 risk = %+v, want %+v", commits[0].Risks[0], want)
	}

	if got := len(commits[1].Risks); got != 0 {
		t.Fatalf("commit 1 risks = %d, want 0 (Two needs nothing)", got)
	}
}

func TestEmitterCarriesFileModeUnchanged(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		mode FileMode
	}{
		{"a plain file", FileMode("100644")},
		{"an executable script", FileMode("100755")},
		{"a symlink", FileMode("120000")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			source := NewMockSource(ctrl)
			source.EXPECT().Read(gomock.Any(), "a.go").Return([]byte("final"), testCase.mode, nil)

			var gotMode FileMode

			sink := NewMockSink(ctrl)
			sink.EXPECT().
				Write(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, _ []byte, mode FileMode) (Handle, error) {
					gotMode = mode

					return "handle", nil
				})

			plan := entities.Plan{Commits: []entities.PlannedCommit{
				{Position: 0, Subject: "feat: One", Decls: []entities.DeclKey{key("a.go", "One")}},
			}}

			// A graph scoped to only what this plan places: checkCoverage
			// refuses a graph declaration the plan never places anywhere.
			graph := entities.NewGraph(
				[]entities.Decl{decl(key("a.go", "One"), entities.Span{Start: 0, End: 3})},
				nil,
				nil,
			)

			commits, err := New(source, growingRenderer(ctrl), sink).
				Emit(context.Background(), plan, graph)
			if err != nil {
				t.Fatalf("Emit() = %v", err)
			}

			if gotMode != testCase.mode {
				t.Fatalf("mode passed to sink = %q, want %q", gotMode, testCase.mode)
			}

			if commits[0].Blobs[0].Mode != testCase.mode {
				t.Fatalf("blob mode = %q, want %q", commits[0].Blobs[0].Mode, testCase.mode)
			}
		})
	}
}

func TestEmitterRefusesAPlanItCannotTrust(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		plan entities.Plan
	}{
		{"an empty plan", entities.Plan{}},
		{
			"a declaration in no commit is a hole in the final tree",
			entities.Plan{
				Commits:  reviewedPlan().Commits,
				Unplaced: []entities.DeclKey{key("z.go", "Lost")},
			},
		},
		{
			"positions are not the dense ordinal sequence the design promises",
			entities.Plan{Commits: []entities.PlannedCommit{
				{Position: 1, Subject: "feat: One", Decls: []entities.DeclKey{key("a.go", "One")}},
			}},
		},
		{
			"the same declaration placed in two commits would duplicate its content",
			entities.Plan{Commits: []entities.PlannedCommit{
				{Position: 0, Subject: "feat: One", Decls: []entities.DeclKey{key("a.go", "One")}},
				{
					Position: 1,
					Subject:  "feat: One again",
					Decls:    []entities.DeclKey{key("a.go", "One")},
				},
			}},
		},
		{
			"a declaration the graph knows about lands in no commit",
			entities.Plan{Commits: []entities.PlannedCommit{
				// sampleGraph() also carries Two, which this plan never places.
				{Position: 0, Subject: "feat: One", Decls: []entities.DeclKey{key("a.go", "One")}},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			_, err := New(NewMockSource(ctrl), NewMockRenderer(ctrl), NewMockSink(ctrl)).
				Emit(context.Background(), testCase.plan, sampleGraph())

			if !errors.Is(err, apperr.ErrIncomplete) {
				t.Fatalf("Emit() = %v, want %v", err, apperr.ErrIncomplete)
			}
		})
	}
}

func TestEmitterRefusesACommitThatChangesNothing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().
		Read(gomock.Any(), gomock.Any()).
		Return([]byte("final"), FileMode("100644"), nil).
		AnyTimes()

	// A renderer that emits nothing stands for a file whose declarations are
	// all still unborn. The commit then changes no file, and an empty commit
	// wearing a feature subject claims work that happened somewhere else.
	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	_, err := New(source, renderer, NewMockSink(ctrl)).
		Emit(context.Background(), reviewedPlan(), sampleGraph())

	if !errors.Is(err, apperr.ErrIncomplete) {
		t.Fatalf("Emit() = %v, want %v", err, apperr.ErrIncomplete)
	}
}

func TestEmitterPropagatesFailures(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	testCases := []struct {
		name  string
		setup func(*gomock.Controller) (*MockSource, *MockRenderer, *MockSink)
	}{
		{
			"a file that cannot be read",
			func(ctrl *gomock.Controller) (*MockSource, *MockRenderer, *MockSink) {
				source := NewMockSource(ctrl)
				source.EXPECT().
					Read(gomock.Any(), gomock.Any()).
					Return(nil, FileMode(""), errBoom)

				return source, NewMockRenderer(ctrl), NewMockSink(ctrl)
			},
		},
		{
			"a file that cannot be rendered",
			func(ctrl *gomock.Controller) (*MockSource, *MockRenderer, *MockSink) {
				source := NewMockSource(ctrl)
				source.EXPECT().
					Read(gomock.Any(), gomock.Any()).
					Return([]byte("final"), FileMode("100644"), nil)

				renderer := NewMockRenderer(ctrl)
				renderer.EXPECT().
					Render(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errBoom)

				return source, renderer, NewMockSink(ctrl)
			},
		},
		{
			"a blob the sink refuses to store",
			func(ctrl *gomock.Controller) (*MockSource, *MockRenderer, *MockSink) {
				source := NewMockSource(ctrl)
				source.EXPECT().
					Read(gomock.Any(), gomock.Any()).
					Return([]byte("final"), FileMode("100644"), nil)

				sink := NewMockSink(ctrl)
				sink.EXPECT().
					Write(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(Handle(""), errBoom)

				return source, growingRenderer(ctrl), sink
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			source, renderer, sink := testCase.setup(ctrl)

			_, err := New(source, renderer, sink).
				Emit(context.Background(), reviewedPlan(), sampleGraph())

			if !errors.Is(err, errBoom) {
				t.Fatalf("Emit() = %v, want it to carry %v", err, errBoom)
			}
		})
	}
}

// A commit that leaves a piece incomplete says so in its subject. Only the
// final tree has to be green; an unmarked incomplete commit is
// indistinguishable from a defect, and a real repository reported 311 declared
// incompletenesses as 311 failures.
func TestEmitMarksACommitThatLeavesAPieceIncomplete(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	early := key("a.go", "One")
	late := key("b.go", "Two")

	graph := entities.NewGraph(
		[]entities.Decl{decl(early, entities.Span{Start: 0, End: 4}), decl(late, entities.Span{Start: 0, End: 4})},
		[]entities.Edge{{Dependent: early, Provider: late, Reason: entities.ReasonReference}},
		nil,
	)

	source := NewMockSource(ctrl)
	source.EXPECT().
		Read(gomock.Any(), gomock.Any()).
		Return([]byte("body"), FileMode("100644"), nil).
		AnyTimes()

	plan := entities.Plan{Commits: []entities.PlannedCommit{
		{Position: 0, Subject: "feat(a): One", Decls: []entities.DeclKey{early}},
		{Position: 1, Subject: "feat(b): Two", Decls: []entities.DeclKey{late}},
	}}

	commits, err := New(source, growingRenderer(ctrl), countingSink(ctrl)).
		Emit(t.Context(), plan, graph)
	if err != nil {
		t.Fatalf("Emit() = %v", err)
	}

	if !commits[0].IsWIP {
		t.Fatal("the commit reaching for something unborn is not marked incomplete")
	}

	if !entities.IsWIPSubject(commits[0].Subject) {
		t.Fatalf("subject = %q, want the marker", commits[0].Subject)
	}

	// The commit that needs nothing later stays unmarked.
	if commits[1].IsWIP || entities.IsWIPSubject(commits[1].Subject) {
		t.Fatalf("a complete commit was marked: %q", commits[1].Subject)
	}
}
