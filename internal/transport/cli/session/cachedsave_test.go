package session

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/plancache"
)

func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()

	var logged bytes.Buffer

	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey || attr.Key == slog.LevelKey {
				return slog.Attr{}
			}

			return attr
		},
	})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	return &logged
}

func TestSaveWarnsOnceWhenTheEntryIsNotKept(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	testCases := []struct {
		name  string
		stub  liveStub
		store plancache.Store
		want  string
	}{
		{"it settled and fits", liveStub{order: cannedOrder()}, plancache.Open(t.TempDir()), ""},
		{
			"it is too large",
			liveStub{order: cannedOrder()},
			plancache.Open(t.TempDir()).Capped(1),
			`msg="the plan cache was not saved: this plan is too large to keep"`,
		},
		{
			"the dir cannot be made",
			liveStub{order: cannedOrder()},
			plancache.Open(filepath.Join(blocker, "x")),
			`msg="the plan cache was not saved" error=`,
		},
		{
			"the plan fails",
			liveStub{order: cannedOrder(), planErr: errors.New("plan")},
			plancache.Open(t.TempDir()),
			`msg="the plan cache was not saved: planning failed" error=plan`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logged := captureWarnings(t)

			counter := &startCounter{live: testCase.stub.pipeline(t)}
			settleThrough(t, begin(t, counter, testCase.store))

			var saves []string

			for line := range strings.SplitSeq(logged.String(), "\n") {
				if strings.Contains(line, "the plan cache was not saved") {
					saves = append(saves, line)
				}
			}

			switch {
			case testCase.want == "" && len(saves) != 0:
				t.Fatalf("logged %q, want no save warning", saves)
			case testCase.want != "" && (len(saves) != 1 || !strings.HasPrefix(saves[0], testCase.want)):
				t.Fatalf("logged %q, want exactly one line starting %q", saves, testCase.want)
			}
		})
	}
}

func TestPlanFailingDuringTheRunSavesNothing(t *testing.T) {
	logged := captureWarnings(t)
	store := plancache.Open(t.TempDir())
	live := liveStub{order: cannedOrder(), planErr: errors.New("plan")}.pipeline(t)
	pl := begin(t, &startCounter{live: live}, store)

	result, _ := pl.Graph(t.Context())
	order, _ := pl.Order(result.Graph)

	if _, err := pl.Plan(result.Graph, order); err == nil {
		t.Fatal("Plan() err = nil, want the stage failure")
	}

	if err := pl.Close(t.Context()); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	if _, err := store.Load(cacheKeyFixture()); !errors.Is(err, plancache.ErrMiss) {
		t.Fatalf("Load() err = %v, want nothing saved after a failed plan", err)
	}

	if !strings.Contains(logged.String(), "planning failed") {
		t.Fatalf("logged %q, want the planning failure named", logged.String())
	}
}

func TestPlanFailingDuringTheRunIsNotPlannedAgainOnClose(t *testing.T) {
	live := NewMockPipeline(gomock.NewController(t))
	live.EXPECT().Graph(gomock.Any()).Return(indexing.Result{Graph: declGraph()}, nil)
	live.EXPECT().Order(gomock.Any()).Return(cannedOrder(), nil)
	live.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(cannedPlan(), errors.New("plan")).Times(1)
	live.EXPECT().Close(gomock.Any()).Return(nil)

	pl := begin(t, &startCounter{live: live}, plancache.Open(t.TempDir()))
	result, _ := pl.Graph(t.Context())
	order, _ := pl.Order(result.Graph)

	if _, err := pl.Plan(result.Graph, order); err == nil {
		t.Fatal("Plan() err = nil, want the stage failure")
	}

	if err := pl.Close(t.Context()); err != nil {
		t.Fatalf("Close() err = %v", err)
	}
}
