package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

func TestDispatchUsagePaths(t *testing.T) {
	t.Parallel()

	unreachable := func(context.Context, bootstrap.Config) (pipeline, error) {
		t.Fatal("a usage path must never start a pipeline")

		return nil, nil
	}

	testCases := []struct {
		name string
		args []string
	}{
		{"no command at all", nil},
		{"an unknown command", []string{"frobnicate", "--repo", "."}},
		{"a command with a usage-invalid flag set", []string{cmdAudit, "--repo", ""}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			err := dispatch(
				context.Background(),
				testCase.args,
				&stdout,
				&stderr,
				runEnv{Start: unreachable},
			)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("err = %v, want it to wrap ErrUsage", err)
			}

			if !strings.Contains(stderr.String(), runUsage) {
				t.Fatalf("stderr = %q, want it to carry the run flags usage", stderr.String())
			}

			if !strings.Contains(stderr.String(), "redigit rebuilds") {
				t.Fatalf("stderr = %q, want it to carry the usage text", stderr.String())
			}

			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want nothing written on a usage path", stdout.String())
			}
		})
	}
}

func TestDispatchRoutesKnownCommands(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		command string
	}{
		{"audit", cmdAudit},
		{"graph", cmdGraph},
		{"plan", cmdPlan},
		{"report", cmdReport},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			pl := NewMockPipeline(ctrl)
			pl.EXPECT().Unseen().Return(nil).AnyTimes()
			pl.EXPECT().Unclaimed().Return(nil).AnyTimes()
			pl.EXPECT().Close(gomock.Any()).Return(nil)

			errRoute := errors.New("routed")
			pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{}, errRoute)

			start := func(context.Context, bootstrap.Config) (pipeline, error) { return pl, nil }

			var stdout, stderr bytes.Buffer

			args := []string{testCase.command, "--repo", "."}

			err := dispatch(context.Background(), args, &stdout, &stderr, runEnv{Start: start})
			if !errors.Is(err, errRoute) {
				t.Fatalf(
					"err = %v, want it to wrap %v (proves the command reached a handler)",
					err,
					errRoute,
				)
			}

			if errors.Is(err, ErrUsage) {
				t.Fatalf("err = %v, a known command must not be treated as a usage error", err)
			}
		})
	}
}

func TestRunDelegatesToRealDispatch(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	err := Run(context.Background(), nil, &stdout, &stderr)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err = %v, want it to wrap ErrUsage", err)
	}
}

func cachedRun(t *testing.T, env runEnv, args ...string) string {
	t.Helper()

	var stdout, stderr bytes.Buffer

	if err := dispatch(t.Context(), args, &stdout, &stderr, env); err != nil {
		t.Fatalf("dispatch(%v) err = %v", args, err)
	}

	return stdout.String()
}

func TestDispatchFromCache(t *testing.T) {
	t.Parallel()

	for _, command := range []string{cmdGraph, cmdPlan} {
		t.Run(command+" is answered without a pipeline", func(t *testing.T) {
			t.Parallel()

			counter := &startCounter{live: settledLive(t)}
			env := cacheTestEnv(t.TempDir(), counter.start)
			args := []string{command, "--repo", ".", "--tags", "integration", "--from-cache"}

			cold := cachedRun(t, env, args...)

			if counter.count != 1 {
				t.Fatalf("cold run started %d pipelines, want 1", counter.count)
			}

			warm := cachedRun(t, env, args...)

			if counter.count != 1 {
				t.Fatalf("warm run started %d pipelines in total, want still 1", counter.count)
			}

			if warm != cold || cold == "" {
				t.Fatalf("warm output differs from cold:\n%s\nvs\n%s", warm, cold)
			}

			other := cachedRun(t, env, command, "--repo", ".", "--tags", "other", "--from-cache")
			if counter.count != 2 || other != cold {
				t.Fatalf("changed tags: %d starts, want a miss and the same output", counter.count)
			}
		})
	}
}

func TestDispatchFromCacheIsOptIn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	counter := &startCounter{live: settledLive(t)}
	env := cacheTestEnv(dir, counter.start)

	cachedRun(t, env, cmdGraph, "--repo", ".")
	cachedRun(t, env, cmdGraph, "--repo", ".")

	if entries, _ := os.ReadDir(dir); counter.count != 2 || len(entries) != 0 {
		t.Fatalf(
			"%d starts and %d cache files, want a cache that is never touched",
			counter.count,
			len(entries),
		)
	}
}

func TestDispatchFromCacheFallbacks(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	testCases := []struct {
		name   string
		dir    string
		mutate func(t *testing.T, env *runEnv)
	}{
		{"a failed save", filepath.Join(blocker, "cache"), func(*testing.T, *runEnv) {}},
		{"a key that cannot be built", t.TempDir(), func(_ *testing.T, env *runEnv) {
			env.Commit = func(context.Context, string, string) (string, error) {
				return "", errors.New("no such rev")
			}
		}},
		{"a corrupt entry", t.TempDir(), func(t *testing.T, env *runEnv) {
			t.Helper()

			cachedRun(t, *env, cmdGraph, "--repo", ".", "--from-cache")

			files, err := filepath.Glob(filepath.Join(env.CacheDir, "*.gob"))
			if err != nil || len(files) != 1 {
				t.Fatalf("cache files = %v, %v, want the one just saved", files, err)
			}

			if err = os.WriteFile(files[0], []byte("junk"), 0o600); err != nil {
				t.Fatalf("corrupt the entry: %v", err)
			}
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			counter := &startCounter{live: settledLive(t)}
			env := cacheTestEnv(testCase.dir, counter.start)
			plain := cachedRun(t, cacheTestEnv(t.TempDir(), counter.start), cmdGraph, "--repo", ".")

			testCase.mutate(t, &env)

			before := counter.count
			got := cachedRun(t, env, cmdGraph, "--repo", ".", "--from-cache")

			if got != plain || counter.count != before+1 {
				t.Fatalf(
					"output or start count wrong: %d new starts, output equal %v",
					counter.count-before,
					got == plain,
				)
			}
		})
	}
}

type startCounter struct {
	count int
	live  pipeline
}

//nolint:ireturn // A fake starter returns the pipeline port, as the real one does.
func (counter *startCounter) start(context.Context, bootstrap.Config) (pipeline, error) {
	counter.count++

	return counter.live, nil
}

func settledLive(t *testing.T) *MockPipeline {
	t.Helper()

	order := entities.Ordering{
		At:     map[entities.DeclKey]entities.Position{declKey("A"): 0},
		Rounds: 1,
	}
	plan := entities.Plan{Commits: []entities.PlannedCommit{
		{
			Subject: "DRAFT: feat: a",
			Shape:   entities.ShapeLayer,
			Decls:   []entities.DeclKey{declKey("A")},
		},
	}}

	pl := NewMockPipeline(gomock.NewController(t))
	pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{Graph: declGraph()}, nil).AnyTimes()
	pl.EXPECT().Order(gomock.Any()).Return(order, nil).AnyTimes()
	pl.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(plan, nil).AnyTimes()
	pl.EXPECT().Unseen().Return(nil).AnyTimes()
	pl.EXPECT().Unclaimed().Return(nil).AnyTimes()
	pl.EXPECT().Close(gomock.Any()).Return(nil).AnyTimes()

	return pl
}

func degradedLive(t *testing.T) *MockPipeline {
	t.Helper()

	order := entities.Ordering{
		At:     map[entities.DeclKey]entities.Position{declKey("A"): 0},
		Rounds: 1,
	}
	result := indexing.Result{
		Graph: declGraph(),
		Blind: []indexing.Blind{{Decl: declKey("A"), Query: "references", Reason: "no answer"}},
		Partial: []indexing.Partial{
			{Broken: 3, Of: 40, Tags: []string{"integration"}},
			{Broken: 1, Of: 12},
		},
	}

	pl := NewMockPipeline(gomock.NewController(t))
	pl.EXPECT().Graph(gomock.Any()).Return(result, nil).AnyTimes()
	pl.EXPECT().Order(gomock.Any()).Return(order, nil).AnyTimes()
	pl.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(entities.Plan{}, nil).AnyTimes()
	pl.EXPECT().
		Unseen().
		Return([]auditing.Unseen{{File: "x.go", Constraint: "//go:build x"}}).
		AnyTimes()
	pl.EXPECT().Unclaimed().Return([]auditing.Unclaimed{{File: "y.bin"}}).AnyTimes()
	pl.EXPECT().Close(gomock.Any()).Return(nil).AnyTimes()

	return pl
}

func TestDispatchFromCacheReplaysEveryWarning(t *testing.T) {
	var logged bytes.Buffer

	previous := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.Attr{}
			}

			return attr
		},
	})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	counter := &startCounter{live: degradedLive(t)}
	env := cacheTestEnv(t.TempDir(), counter.start)
	args := []string{cmdPlan, "--repo", ".", "--from-cache"}

	cachedRun(t, env, args...)

	miss := logged.String()

	logged.Reset()
	cachedRun(t, env, args...)

	if counter.count != 1 {
		t.Fatalf("the second run started %d pipelines in total, want a hit", counter.count)
	}

	if hit := logged.String(); hit != miss || strings.Count(miss, "type-check") != 2 {
		t.Fatalf("a hit says\n%s\nwant exactly what the miss said\n%s", hit, miss)
	}
}

func keyForbiddenEnv(dir string, keyed *bool) runEnv {
	built := func() string {
		*keyed = true

		return ""
	}

	return runEnv{
		Start:     func(context.Context, bootstrap.Config) (pipeline, error) { return nil, errors.New("no") },
		CacheDir:  dir,
		BuildID:   func() (string, error) { return built(), nil },
		Commit:    func(context.Context, string, string) (string, error) { return built(), nil },
		Digest:    func(string) (string, error) { return built(), nil },
		Toolchain: func(context.Context, string, string) (string, error) { return built(), nil },
	}
}

func TestDispatchChecksTheCommandBeforeAnySideEffect(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	profile := filepath.Join(dir, "p.pprof")

	var keyed bool

	args := []string{"bogus", "--repo", ".", "--cpuprofile", profile, "--from-cache"}

	err := dispatch(t.Context(), args, io.Discard, io.Discard, keyForbiddenEnv(dir, &keyed))
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("dispatch() err = %v, want ErrUsage", err)
	}

	if _, statErr := os.Stat(profile); !errors.Is(statErr, os.ErrNotExist) || keyed {
		t.Fatalf("a bogus command made the profile (%v) or built a key (%v)", statErr, keyed)
	}
}

func TestDispatchTimesTheLaunchAHitMakesLater(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		live := settledLive(t)
		live.EXPECT().Audit(gomock.Any(), gomock.Any()).Return(auditing.Result{}, nil).AnyTimes()

		counter := &startCounter{live: live}
		env := cacheTestEnv(t.TempDir(), counter.start)
		args := []string{cmdAudit, "--repo", ".", "--from-cache", "--timings"}

		cachedRun(t, env, cmdAudit, "--repo", ".", "--from-cache")

		env.Start = func(ctx context.Context, cfg bootstrap.Config) (pipeline, error) {
			synctest.Sleep(700 * time.Millisecond)

			return counter.start(ctx, cfg)
		}

		var stderr bytes.Buffer

		if err := dispatch(t.Context(), args, io.Discard, &stderr, env); err != nil {
			t.Fatalf("dispatch() err = %v", err)
		}

		var line struct {
			Timings map[string]float64 `json:"timings"`
		}

		err := json.Unmarshal(stderr.Bytes(), &line)
		if err != nil || line.Timings["start"] != 700 {
			t.Fatalf("stderr = %q (%v), want start = 700 ms", stderr.String(), err)
		}
	})
}

func TestDispatchWarnsThatChunksIgnoresFromCache(t *testing.T) {
	const warning = "--from-cache has no effect on chunks"

	testCases := []struct {
		name     string
		args     []string
		wantWarn bool
	}{
		{"chunks with the flag", []string{cmdChunks, "--repo", ".", "--from-cache"}, true},
		{"chunks without it", []string{cmdChunks, "--repo", "."}, false},
		{"plan with the flag", []string{cmdPlan, "--repo", ".", "--from-cache"}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var logged bytes.Buffer

			previous := slog.Default()

			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })

			var keyed bool

			env := keyForbiddenEnv(t.TempDir(), &keyed)
			_ = dispatch(t.Context(), testCase.args, io.Discard, io.Discard, env)

			if got := strings.Contains(logged.String(), warning); got != testCase.wantWarn {
				t.Fatalf("log %q: warned = %v, want %v", logged.String(), got, testCase.wantWarn)
			}
		})
	}
}
