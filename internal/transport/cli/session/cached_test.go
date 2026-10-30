package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/infra/plancache"
)

func declKey() entities.DeclKey {
	return entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "A"}
}

func declGraph() entities.Graph {
	return entities.NewGraph([]entities.Decl{{Key: declKey(), Unit: "a"}}, nil, nil)
}

func cannedOrder() entities.Ordering {
	return entities.Ordering{
		At:     map[entities.DeclKey]entities.Position{declKey(): 0},
		Rounds: 1,
	}
}

func cannedPlan() entities.Plan {
	return entities.Plan{Commits: []entities.PlannedCommit{
		{
			Subject: "DRAFT: feat: a",
			Shape:   entities.ShapeLayer,
			Decls:   []entities.DeclKey{declKey()},
		},
	}}
}

type liveStub struct {
	graphErr error
	order    entities.Ordering
	orderErr error
	planErr  error
}

func (stub liveStub) pipeline(t *testing.T) *MockPipeline {
	t.Helper()

	pl := NewMockPipeline(gomock.NewController(t))
	pl.EXPECT().
		Graph(gomock.Any()).
		Return(indexing.Result{Graph: declGraph()}, stub.graphErr).
		AnyTimes()
	pl.EXPECT().Order(gomock.Any()).Return(stub.order, stub.orderErr).AnyTimes()
	pl.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(cannedPlan(), stub.planErr).AnyTimes()
	pl.EXPECT().Unseen().Return([]auditing.Unseen{{File: "x.go"}}).AnyTimes()
	pl.EXPECT().Unclaimed().Return([]auditing.Unclaimed{{File: "y.bin"}}).AnyTimes()
	pl.EXPECT().Close(gomock.Any()).Return(nil).AnyTimes()

	return pl
}

type startCounter struct {
	count int
	live  Pipeline
	err   error
}

//nolint:ireturn // A fake Starter returns the Pipeline port, as the real one does.
func (counter *startCounter) start(context.Context, bootstrap.Config) (Pipeline, error) {
	counter.count++

	return counter.live, counter.err
}

func cacheKeyFixture() string { return strings.Repeat("ab", 32) }

//nolint:ireturn // The Starter's product is the Pipeline port.
func begin(t *testing.T, counter *startCounter, store plancache.Store) Pipeline {
	t.Helper()

	pl, err := CachingStart(
		counter.start,
		store,
		resolvedFixture(),
		nil,
	)(
		t.Context(),
		bootstrap.Config{},
	)
	if err != nil {
		t.Fatalf("start err = %v", err)
	}

	return pl
}

func settleThrough(t *testing.T, pl Pipeline) {
	t.Helper()

	result, err := pl.Graph(t.Context())
	if err == nil {
		_, _ = pl.Order(result.Graph)
	}

	if err = pl.Close(t.Context()); err != nil {
		t.Fatalf("Close() err = %v", err)
	}
}

func seededStore(t *testing.T, stub liveStub) plancache.Store {
	t.Helper()

	store := plancache.Open(t.TempDir())
	settleThrough(t, begin(t, &startCounter{live: stub.pipeline(t)}, store))

	return store
}

func TestCachingStartSavesWhatTheRunSettled(t *testing.T) {
	t.Parallel()

	store := seededStore(t, liveStub{order: cannedOrder()})

	entry, err := store.Load(cacheKeyFixture())
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}

	if len(entry.Decls) != 1 || entry.Ordering.Rounds != 1 || len(entry.Plan.Commits) != 1 {
		t.Fatalf("saved entry = %+v, want the graph, ordering and plan", entry)
	}

	if entry.Unseen[0].File != "x.go" || entry.Unclaimed[0].File != "y.bin" {
		t.Fatalf("saved warnings = %v, %v", entry.Unseen, entry.Unclaimed)
	}
}

func TestCachingStartAnswersAHitWithoutStarting(t *testing.T) {
	t.Parallel()

	store := seededStore(t, liveStub{order: cannedOrder()})
	counter := &startCounter{}
	hit := begin(t, counter, store)

	result, _ := hit.Graph(t.Context())
	order, orderErr := hit.Order(result.Graph)
	plan, _ := hit.Plan(result.Graph, order)

	if result.Graph.Len() != 1 || orderErr != nil || !reflect.DeepEqual(order, cannedOrder()) {
		t.Fatalf("hit answered %v, %v, %v", result, order, orderErr)
	}

	if !reflect.DeepEqual(plan, cannedPlan()) || hit.Unseen()[0].File != "x.go" ||
		hit.Unclaimed()[0].File != "y.bin" {
		t.Fatalf("hit answered plan %v and warnings %v, %v", plan, hit.Unseen(), hit.Unclaimed())
	}

	if err := hit.Close(t.Context()); err != nil || counter.count != 0 {
		t.Fatalf("Close() = %v after %d starts, want a quiet hit", err, counter.count)
	}
}

func TestCachingStartKeepsNothingUnlessSettled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		stub liveStub
	}{
		{"the graph failed", liveStub{graphErr: errors.New("index"), order: cannedOrder()}},
		{
			"the ordering never converged",
			liveStub{order: cannedOrder(), orderErr: ordering.ErrNotConverged},
		},
		{"the ordering holds no positions", liveStub{order: entities.Ordering{Rounds: 1}}},
		{
			"the fixed point ran no rounds",
			liveStub{order: entities.Ordering{At: cannedOrder().At}},
		},
		{"the plan failed", liveStub{order: cannedOrder(), planErr: errors.New("plan")}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			store := seededStore(t, testCase.stub)

			if _, err := store.Load(cacheKeyFixture()); !errors.Is(err, plancache.ErrMiss) {
				t.Fatalf("Load() err = %v, want nothing saved", err)
			}
		})
	}
}

func TestCachingStartReplaysTheOrderingComplaint(t *testing.T) {
	t.Parallel()

	complaint := errors.New("order: a cycle crosses files")
	store := seededStore(t, liveStub{order: cannedOrder(), orderErr: complaint})
	hit := begin(t, &startCounter{}, store)

	if _, got := hit.Order(entities.Graph{}); got == nil || got.Error() != complaint.Error() {
		t.Fatalf("hit Order() err = %v, want the saved complaint %q", got, complaint)
	}
}

func TestCachingStartMissCases(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	blocker := filepath.Join(t.TempDir(), "blocker")

	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	corruptDir := t.TempDir()
	junk := filepath.Join(corruptDir, cacheKeyFixture()+".gob")

	if err := os.WriteFile(junk, []byte("junk"), 0o600); err != nil {
		t.Fatalf("write junk entry: %v", err)
	}

	testCases := []struct {
		name      string
		store     plancache.Store
		wantSaved bool
	}{
		{"a corrupt entry is a miss", plancache.Open(corruptDir), true},
		{"a failed save is not a failure", plancache.Open(filepath.Join(blocker, "x")), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			counter := &startCounter{live: liveStub{order: cannedOrder()}.pipeline(t)}
			settleThrough(t, begin(t, counter, testCase.store))

			_, loadErr := testCase.store.Load(cacheKeyFixture())
			if (loadErr == nil) != testCase.wantSaved {
				t.Fatalf("Load() err = %v, want saved = %v", loadErr, testCase.wantSaved)
			}
		})
	}

	failing := &startCounter{err: errBoom}
	store := plancache.Open(t.TempDir())

	if _, err := CachingStart(
		failing.start,
		store,
		resolvedFixture(),
		nil,
	)(
		t.Context(),
		bootstrap.Config{},
	); !errors.Is(
		err,
		errBoom,
	) {
		t.Fatalf("start err = %v, want the start failure", err)
	}
}

func errOnly[Value any](_ Value, err error) error { return err }

func resolvedFixture() Resolved {
	return Resolved{Key: cacheKeyFixture(), Commit: "0123abc"}
}

func TestCachingStartPinsTheRunToTheCommitOfTheKey(t *testing.T) {
	t.Parallel()

	var seen []string

	pl := liveStub{order: cannedOrder()}.pipeline(t)
	inner := func(_ context.Context, cfg bootstrap.Config) (Pipeline, error) {
		seen = append(seen, cfg.Rev)

		return pl, nil
	}

	store := plancache.Open(t.TempDir())
	cfg := bootstrap.Config{Rev: "main"}

	live, err := CachingStart(inner, store, resolvedFixture(), nil)(t.Context(), cfg)
	if err != nil {
		t.Fatalf("start err = %v", err)
	}

	settleThrough(t, live)

	if !slices.Equal(seen, []string{"0123abc"}) {
		t.Fatalf("the pipeline read revisions %q, want only the commit of the key", seen)
	}

	entry, err := store.Load(cacheKeyFixture())
	if err != nil || entry.Commit != "0123abc" {
		t.Fatalf("saved entry = %q, %v, want the commit of the key", entry.Commit, err)
	}

	moved := Resolved{Key: cacheKeyFixture(), Commit: "fedcba9"}

	hit, err := CachingStart(inner, store, moved, nil)(t.Context(), cfg)
	if err != nil {
		t.Fatalf("start err = %v", err)
	}

	settleThrough(t, hit)

	if !slices.Equal(seen, []string{"0123abc", "fedcba9"}) {
		t.Fatalf("revisions read = %q, want an entry of another commit to be a miss", seen)
	}
}
