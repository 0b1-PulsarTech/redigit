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
					Write(gomock.Any(), gomock.Any(), gomock.Any()).
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
