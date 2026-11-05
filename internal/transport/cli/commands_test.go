package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// failWriter always fails, so a test can prove a command's output-write
// error path without a real broken pipe.
type failWriter struct{ err error }

func (w failWriter) Write([]byte) (int, error) { return 0, w.err }

func declKey(name string) entities.DeclKey {
	return entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: name}
}

func declGraph() entities.Graph {
	return entities.NewGraph([]entities.Decl{{Key: declKey("A"), Unit: "a"}}, nil, nil)
}

func settledStart(ctrl *gomock.Controller, graphErr error) (starter, *MockPipeline) {
	pl := NewMockPipeline(ctrl)
	pl.EXPECT().Unseen().Return(nil).AnyTimes()
	pl.EXPECT().Unclaimed().Return(nil).AnyTimes()

	if graphErr != nil {
		pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{}, graphErr)
	} else {
		pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{Graph: declGraph()}, nil)
		pl.EXPECT().Order(gomock.Any()).Return(entities.Ordering{
			At:     map[entities.DeclKey]entities.Position{declKey("A"): 0},
			Rounds: 1,
		}, nil)
	}

	pl.EXPECT().Close(gomock.Any()).Return(nil)

	start := func(context.Context, bootstrap.Config) (pipeline, error) { return pl, nil }

	return start, pl
}

func TestRunAudit(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	t.Run("the graph stage failing fails the command", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, _ := settledStart(ctrl, errBoom)

		var out bytes.Buffer

		err := runAudit(context.Background(), bootstrap.Config{}, &out, start)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want it to wrap %v", err, errBoom)
		}
	})

	t.Run("the audit stage failing fails the command", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, pl := settledStart(ctrl, nil)
		pl.EXPECT().Audit(gomock.Any(), gomock.Any()).Return(auditing.Result{}, errBoom)

		var out bytes.Buffer

		err := runAudit(context.Background(), bootstrap.Config{}, &out, start)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want it to wrap %v", err, errBoom)
		}
	})

	t.Run("a write failure surfaces instead of being swallowed", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, pl := settledStart(ctrl, nil)
		pl.EXPECT().Audit(gomock.Any(), gomock.Any()).Return(auditing.Result{}, nil)

		errWrite := errors.New("disk full")

		err := runAudit(context.Background(), bootstrap.Config{}, failWriter{err: errWrite}, start)
		if !errors.Is(err, errWrite) {
			t.Fatalf("err = %v, want it to wrap %v", err, errWrite)
		}
	})
}

// TestRunAuditSuccess is split out from TestRunAudit's error-path table so
// neither function grows past the project's function-length budget.
func TestRunAuditSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	start, pl := settledStart(ctrl, nil)
	pl.EXPECT().Audit(gomock.Any(), gomock.Any()).Return(auditing.Result{
		Report: entities.Report{
			Integrity: []entities.Measure{
				{Name: "placed", Value: 1, Total: 1, Excluded: []string{"x"}},
			},
		},
		Drifts: []auditing.Drift{
			{Decl: declKey("A"), Reader: declKey("B"), Reason: entities.ReasonReference},
		},
	}, nil)

	var out bytes.Buffer

	cfg := bootstrap.Config{Repo: "/repo", Rev: "HEAD"}
	if err := runAudit(context.Background(), cfg, &out, start); err != nil {
		t.Fatalf("runAudit() = %v", err)
	}

	var got auditDTO
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Repo != "/repo" || got.Declarations != 1 || len(got.Integrity) != 1 ||
		len(got.Drift) != 1 || got.Integrity[0].Percent != 100 {
		t.Fatalf("got = %+v", got)
	}
}

func TestRunGraph(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	t.Run("the graph stage failing fails the command", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, _ := settledStart(ctrl, errBoom)

		var out bytes.Buffer

		err := runGraph(context.Background(), bootstrap.Config{}, &out, start)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want it to wrap %v", err, errBoom)
		}
	})

	t.Run("a write failure surfaces instead of being swallowed", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, _ := settledStart(ctrl, nil)

		errWrite := errors.New("disk full")

		err := runGraph(context.Background(), bootstrap.Config{}, failWriter{err: errWrite}, start)
		if !errors.Is(err, errWrite) {
			t.Fatalf("err = %v, want it to wrap %v", err, errWrite)
		}
	})

	t.Run("success emits decoded, well-shaped JSON", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, _ := settledStart(ctrl, nil)

		var out bytes.Buffer

		cfg := bootstrap.Config{Repo: "/repo", Rev: "HEAD"}
		if err := runGraph(context.Background(), cfg, &out, start); err != nil {
			t.Fatalf("runGraph() = %v", err)
		}

		var got graphDTO
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if got.Repo != "/repo" || len(got.Decls) != 1 || got.Decls[0].Key != declKey("A").String() {
			t.Fatalf("got = %+v", got)
		}
	})
}

func TestRunPlan(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	t.Run("the graph stage failing fails the command", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, _ := settledStart(ctrl, errBoom)

		var out bytes.Buffer

		err := runPlan(context.Background(), bootstrap.Config{}, &out, start)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want it to wrap %v", err, errBoom)
		}
	})

	t.Run("the plan stage failing fails the command", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, pl := settledStart(ctrl, nil)
		pl.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(entities.Plan{}, errBoom)

		var out bytes.Buffer

		err := runPlan(context.Background(), bootstrap.Config{}, &out, start)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want it to wrap %v", err, errBoom)
		}
	})

	t.Run("a write failure surfaces instead of being swallowed", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, pl := settledStart(ctrl, nil)
		pl.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(entities.Plan{}, nil)

		errWrite := errors.New("disk full")

		err := runPlan(context.Background(), bootstrap.Config{}, failWriter{err: errWrite}, start)
		if !errors.Is(err, errWrite) {
			t.Fatalf("err = %v, want it to wrap %v", err, errWrite)
		}
	})

	t.Run("success emits decoded, well-shaped JSON", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start, pl := settledStart(ctrl, nil)
		pl.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(entities.Plan{
			Commits: []entities.PlannedCommit{
				{Position: 0, Subject: "DRAFT: x", Decls: []entities.DeclKey{declKey("A")}},
			},
			Unplaced: []entities.DeclKey{declKey("B")},
		}, nil)

		var out bytes.Buffer

		cfg := bootstrap.Config{Repo: "/repo", Rev: "HEAD"}
		if err := runPlan(context.Background(), cfg, &out, start); err != nil {
			t.Fatalf("runPlan() = %v", err)
		}

		var got planDTO
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if len(got.Commits) != 1 || got.Commits[0].Subject != "DRAFT: x" || len(got.Unplaced) != 1 {
			t.Fatalf("got = %+v", got)
		}
	})
}

// TestWarnBlind pins that a blind spot is logged (not dropped) when the
// resolver could not answer, and that nothing is logged when there is none.
// It runs outside t.Parallel(): it swaps the package-global slog default for
// the duration of the call, and that state is process-wide.
func TestWarnBlind(t *testing.T) {
	testCases := []struct {
		name      string
		blind     []indexing.Blind
		wantLines int
	}{
		{"no blind spot logs nothing", nil, 0},
		{
			"a blind spot is logged",
			[]indexing.Blind{{Decl: declKey("A"), Query: "references", Reason: "timed out"}},
			1,
		},
	}

	for _, testCase := range testCases {
		var buf bytes.Buffer

		restore := slog.Default()
		slog.SetDefault(
			slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})),
		)

		warnBlind(indexing.Result{Graph: declGraph(), Blind: testCase.blind})

		slog.SetDefault(restore)

		got := 0
		if buf.Len() > 0 {
			got = 1
		}

		if got != testCase.wantLines {
			t.Errorf(
				"%s: logged = %d lines, want %d (output: %q)",
				testCase.name,
				got,
				testCase.wantLines,
				buf.String(),
			)
		}
	}
}

var _ io.Writer = failWriter{}

func TestRunGraphIgnoresInputOrder(t *testing.T) {
	t.Parallel()

	names := []string{"A", "B", "C", "D"}
	decls := make([]entities.Decl, 0, len(names))
	edges := make([]entities.Edge, 0, len(names))
	at := map[entities.DeclKey]entities.Position{}

	for index, name := range names {
		decls = append(decls, entities.Decl{Key: declKey(name), Unit: "a"})
		edges = append(edges, entities.Edge{
			Dependent: declKey(name),
			Provider:  declKey(names[(index+1)%len(names)]),
			Reason:    entities.ReasonReference,
		})
		at[declKey(name)] = entities.Position(index)
	}

	render := func(reverse bool) string {
		forward, backward := slices.Clone(decls), slices.Clone(edges)
		if reverse {
			slices.Reverse(forward)
			slices.Reverse(backward)
		}

		ctrl := gomock.NewController(t)
		pl := NewMockPipeline(ctrl)
		pl.EXPECT().Unseen().Return(nil).AnyTimes()
		pl.EXPECT().Unclaimed().Return(nil).AnyTimes()
		pl.EXPECT().Graph(gomock.Any()).Return(
			indexing.Result{Graph: entities.NewGraph(forward, backward, nil)}, nil,
		)
		pl.EXPECT().Order(gomock.Any()).Return(entities.Ordering{At: at, Rounds: 1}, nil)
		pl.EXPECT().Close(gomock.Any()).Return(nil)

		start := func(context.Context, bootstrap.Config) (pipeline, error) { return pl, nil }

		var out bytes.Buffer
		if err := runGraph(context.Background(), bootstrap.Config{}, &out, start); err != nil {
			t.Fatalf("runGraph() = %v", err)
		}

		return out.String()
	}

	testCases := []struct {
		name string
		got  string
		want string
	}{
		{"reversed inputs print the same bytes", render(true), render(false)},
		{"a second run prints the same bytes", render(false), render(false)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.got != testCase.want {
				t.Fatalf("output differs:\n%s\nwant:\n%s", testCase.got, testCase.want)
			}
		})
	}
}
