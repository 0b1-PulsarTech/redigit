package session

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
)

// TestWithPipeline covers the three things withPipeline can do: fail to
// start, run the caller's function and hand back its error, or succeed and
// still close the pipeline it started.
func TestWithPipeline(t *testing.T) {
	t.Parallel()

	t.Run("a start failure is wrapped and run is never called", func(t *testing.T) {
		t.Parallel()

		errStart := errors.New("gopls would not launch")
		start := func(context.Context, bootstrap.Config) (Pipeline, error) {
			return nil, errStart
		}

		called := false

		err := WithPipeline(context.Background(), bootstrap.Config{}, start, func(Pipeline) error {
			called = true

			return nil
		})

		if !errors.Is(err, errStart) {
			t.Fatalf("err = %v, want it to wrap %v", err, errStart)
		}

		if called {
			t.Fatal("run must not be called when start fails")
		}
	})

	t.Run("the pipeline is closed even when run fails", func(t *testing.T) {
		t.Parallel()

		errRun := errors.New("boom")

		ctrl := gomock.NewController(t)
		pl := NewMockPipeline(ctrl)
		pl.EXPECT().Unseen().Return(nil).AnyTimes()
		pl.EXPECT().Unclaimed().Return(nil).AnyTimes()
		pl.EXPECT().Close(gomock.Any()).Return(nil)

		start := func(context.Context, bootstrap.Config) (Pipeline, error) { return pl, nil }

		err := WithPipeline(context.Background(), bootstrap.Config{}, start, func(Pipeline) error {
			return errRun
		})

		if !errors.Is(err, errRun) {
			t.Fatalf("err = %v, want it to wrap %v", err, errRun)
		}
	})

	t.Run("success closes the pipeline too", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		pl := NewMockPipeline(ctrl)
		pl.EXPECT().Unseen().Return(nil).AnyTimes()
		pl.EXPECT().Unclaimed().Return(nil).AnyTimes()
		pl.EXPECT().Close(gomock.Any()).Return(nil)

		start := func(context.Context, bootstrap.Config) (Pipeline, error) { return pl, nil }

		err := WithPipeline(context.Background(), bootstrap.Config{}, start, func(Pipeline) error {
			return nil
		})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})
}
