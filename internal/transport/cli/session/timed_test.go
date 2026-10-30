package session

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/profiling"
)

func TestTimedStartPassesTheStartFailureThrough(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	failing := func(context.Context, bootstrap.Config) (Pipeline, error) { return nil, errBoom }

	pl, err := TimedStart(failing, profiling.NewTimings())(t.Context(), bootstrap.Config{})
	if !errors.Is(err, errBoom) || pl != nil {
		t.Fatalf("start = %v, %v, want nil and the cause", pl, err)
	}
}

func TestTimedPipelineDelegatesEveryStage(t *testing.T) {
	t.Parallel()

	graph := declGraph()
	order := entities.Ordering{Rounds: 2}
	plan := entities.Plan{Unplaced: []entities.DeclKey{declKey()}}

	inner := NewMockPipeline(gomock.NewController(t))
	inner.EXPECT().Births(gomock.Any()).Return(indexing.Births{}, nil)
	inner.EXPECT().Graph(gomock.Any()).Return(indexing.Result{Graph: graph}, nil)
	inner.EXPECT().Order(graph).Return(order, nil)
	inner.EXPECT().Plan(graph, order).Return(plan, nil)
	inner.EXPECT().Audit(graph, order).Return(auditing.Result{}, nil)
	inner.EXPECT().
		Emit(gomock.Any(), plan, graph, nil).
		Return([]emitting.Commit{{Subject: "s"}}, nil)
	inner.EXPECT().Close(gomock.Any()).Return(nil)
	inner.EXPECT().Unseen().Return([]auditing.Unseen{{File: "x.go"}})
	inner.EXPECT().Unclaimed().Return([]auditing.Unclaimed{{File: "y.bin"}})

	timings := profiling.NewTimings()
	start := func(context.Context, bootstrap.Config) (Pipeline, error) { return inner, nil }

	timed, err := TimedStart(start, timings)(t.Context(), bootstrap.Config{})
	if err != nil {
		t.Fatalf("start err = %v", err)
	}

	ctx := t.Context()

	calls := []struct {
		stage string
		call  func() error
	}{
		{"births", func() error { return errOnly(timed.Births(ctx)) }},
		{"graph", func() error { return errOnly(timed.Graph(ctx)) }},
		{"order", func() error { return errOnly(timed.Order(graph)) }},
		{"plan", func() error { return errOnly(timed.Plan(graph, order)) }},
		{"audit", func() error { return errOnly(timed.Audit(graph, order)) }},
		{"emit", func() error { return errOnly(timed.Emit(ctx, plan, graph, nil)) }},
		{"close", func() error { return timed.Close(ctx) }},
	}

	for _, call := range calls {
		if err = call.call(); err != nil {
			t.Fatalf("%s err = %v", call.stage, err)
		}
	}

	if got := timed.Unseen(); len(got) != 1 || got[0].File != "x.go" {
		t.Fatalf("Unseen() = %v", got)
	}

	if got := timed.Unclaimed(); len(got) != 1 || got[0].File != "y.bin" {
		t.Fatalf("Unclaimed() = %v", got)
	}

	var out bytes.Buffer
	if err = timings.Write(&out); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	if !strings.Contains(out.String(), `"start":`) {
		t.Errorf("timings %s lack the start stage", out.String())
	}

	for _, call := range calls {
		if !strings.Contains(out.String(), `"`+call.stage+`":`) {
			t.Errorf("timings %s lack the %s stage", out.String(), call.stage)
		}
	}
}
