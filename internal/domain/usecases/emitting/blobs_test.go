package emitting

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestRenderTouchedReadsAPathOnceAcrossCalls(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().
		Read(gomock.Any(), "a.go").
		Return([]byte("final"), FileMode("100644"), nil).
		Times(1)

	emitter := New(source, growingRenderer(ctrl), countingSink(ctrl))
	born := map[string][]entities.DeclKey{"a.go": {key("a.go", "One")}}
	cache := map[string]sourceFile{}

	if _, err := emitter.renderTouched(
		context.Background(),
		map[string]bool{"a.go": true},
		born,
		cache,
	); err != nil {
		t.Fatalf("renderTouched() first call = %v", err)
	}

	born["a.go"] = append(born["a.go"], key("a.go", "Two"))

	if _, err := emitter.renderTouched(
		context.Background(),
		map[string]bool{"a.go": true},
		born,
		cache,
	); err != nil {
		t.Fatalf("renderTouched() second call = %v", err)
	}
}

func TestRenderTouchedOmitsAFileThatRendersEmpty(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), "a.go").Return([]byte("final"), FileMode("100644"), nil)

	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	emitter := New(source, renderer, NewMockSink(ctrl))

	blobs, err := emitter.renderTouched(
		context.Background(),
		map[string]bool{"a.go": true},
		map[string][]entities.DeclKey{"a.go": {key("a.go", "One")}},
		map[string]sourceFile{},
	)
	if err != nil {
		t.Fatalf("renderTouched() = %v", err)
	}

	if len(blobs) != 0 {
		t.Fatalf("blobs = %v, want none for a file whose declarations are all unborn", blobs)
	}
}

func TestRenderTouchedPropagatesFailures(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	testCases := []struct {
		name  string
		setup func(*gomock.Controller) (*MockSource, *MockRenderer, *MockSink)
	}{
		{
			"the read fails",
			func(ctrl *gomock.Controller) (*MockSource, *MockRenderer, *MockSink) {
				source := NewMockSource(ctrl)
				source.EXPECT().Read(gomock.Any(), gomock.Any()).Return(nil, FileMode(""), errBoom)

				return source, NewMockRenderer(ctrl), NewMockSink(ctrl)
			},
		},
		{
			"the render fails",
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
			"the sink refuses the blob",
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

			_, err := New(source, renderer, sink).renderTouched(
				context.Background(),
				map[string]bool{"a.go": true},
				map[string][]entities.DeclKey{"a.go": {key("a.go", "One")}},
				map[string]sourceFile{},
			)

			if !errors.Is(err, errBoom) {
				t.Fatalf("renderTouched() = %v, want it to carry %v", err, errBoom)
			}
		})
	}
}

// TestSinkReceivesThePath pins the argument a directory sink cannot work
// without. A content-addressed sink ignores it, so nothing here fails loudly
// when it goes missing - the path-addressed implementation simply cannot be
// written at all, and a port that serves only one of its two implementations
// is not a seam.
func TestSinkReceivesThePath(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().
		Read(gomock.Any(), gomock.Any()).
		Return([]byte("final"), FileMode("100755"), nil).
		AnyTimes()

	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().
		Render(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte("rendered"), nil).
		AnyTimes()

	seen := map[string]FileMode{}

	sink := NewMockSink(ctrl)
	sink.EXPECT().
		Write(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, path string, _ []byte, mode FileMode) (Handle, error) {
			seen[path] = mode

			return Handle("h"), nil
		}).
		AnyTimes()

	emitter := New(source, renderer, sink)

	born := map[string][]entities.DeclKey{
		"a/one.go": {{File: "a/one.go", Kind: entities.KindFunc, Name: "One"}},
		"b/two.go": {{File: "b/two.go", Kind: entities.KindFunc, Name: "Two"}},
	}

	if _, err := emitter.renderTouched(
		context.Background(),
		map[string]bool{"a/one.go": true, "b/two.go": true},
		born,
		map[string]sourceFile{},
	); err != nil {
		t.Fatalf("renderTouched() = %v", err)
	}

	for _, want := range []string{"a/one.go", "b/two.go"} {
		if mode, got := seen[want]; !got {
			t.Errorf("the sink never saw path %q", want)
		} else if mode != "100755" {
			t.Errorf("path %q arrived with mode %q, want 100755", want, mode)
		}
	}
}

// git tracks an empty file, so a path that is empty in the FINAL tree has to
// be written even though it renders as nothing. Skipping it leaves the final
// tree short by one path, and the tree invariant catches that only at the very
// end, once every explanation is gone.
func TestEmitterWritesAFileThatIsEmptyInTheFinalTree(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), "database/go.sum").Return([]byte{}, FileMode("100644"), nil)

	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().
		Render(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte{}, nil).
		AnyTimes()

	var written []string

	sink := NewMockSink(ctrl)
	sink.EXPECT().
		Write(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, path string, _ []byte, _ FileMode) (Handle, error) {
			written = append(written, path)

			return Handle("empty"), nil
		}).
		AnyTimes()

	key := entities.DeclKey{
		File: "database/go.sum",
		Kind: entities.KindOther,
		Name: "<file>",
	}
	graph := entities.NewGraph(
		[]entities.Decl{{Key: key, Unit: "database", Span: entities.Span{}}},
		nil,
		nil,
	)
	plan := entities.Plan{Commits: []entities.PlannedCommit{
		{Position: 0, Subject: "chore: the sum file", Decls: []entities.DeclKey{key}},
	}}

	commits, err := New(source, renderer, sink).Emit(t.Context(), plan, graph)
	if err != nil {
		t.Fatalf("Emit() = %v", err)
	}

	if len(written) != 1 || written[0] != "database/go.sum" {
		t.Fatalf("the empty file reached the sink %d time(s): %v", len(written), written)
	}

	if len(commits) != 1 || len(commits[0].Blobs) != 1 {
		t.Fatalf("the commit carries %d blob(s), want the empty file", len(commits[0].Blobs))
	}
}
