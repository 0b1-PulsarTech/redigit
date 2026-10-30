package session

import (
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

func TestCachingStartStartsLazilyOnAHit(t *testing.T) {
	t.Parallel()

	store := seededStore(t, liveStub{order: cannedOrder()})

	live := NewMockPipeline(gomock.NewController(t))
	live.EXPECT().Births(gomock.Any()).Return(indexing.Births{}, nil)
	live.EXPECT().Audit(gomock.Any(), gomock.Any()).Return(auditing.Result{}, nil)
	live.EXPECT().Emit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
	live.EXPECT().Close(gomock.Any()).Return(nil)

	counter := &startCounter{live: live}
	hit := begin(t, counter, store)

	if counter.count != 0 {
		t.Fatalf("%d starts before any stage needed one", counter.count)
	}

	if _, err := hit.Births(t.Context()); err != nil {
		t.Fatalf("Births() err = %v", err)
	}

	if _, err := hit.Audit(declGraph(), cannedOrder()); err != nil {
		t.Fatalf("Audit() err = %v", err)
	}

	if _, err := hit.Emit(t.Context(), cannedPlan(), declGraph(), nil); err != nil {
		t.Fatalf("Emit() err = %v", err)
	}

	if err := hit.Close(t.Context()); err != nil || counter.count != 1 {
		t.Fatalf("Close() = %v after %d starts, want one start reused", err, counter.count)
	}
}

func TestCachedPipelineLazyStartFailure(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	store := seededStore(t, liveStub{order: cannedOrder()})
	hit := begin(t, &startCounter{err: errBoom}, store)

	if _, got := hit.Births(t.Context()); !errors.Is(got, errBoom) {
		t.Fatalf("Births() err = %v, want the start failure", got)
	}

	if _, got := hit.Audit(declGraph(), cannedOrder()); !errors.Is(got, errBoom) {
		t.Fatalf("Audit() err = %v, want the start failure", got)
	}

	if _, got := hit.Emit(t.Context(), cannedPlan(), declGraph(), nil); !errors.Is(got, errBoom) {
		t.Fatalf("Emit() err = %v, want the start failure", got)
	}

	if got := hit.Close(t.Context()); got != nil {
		t.Fatalf("Close() err = %v, want nil with nothing started", got)
	}
}
