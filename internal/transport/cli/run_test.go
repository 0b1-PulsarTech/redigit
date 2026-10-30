package cli

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/plancache"
	"github.com/wrapped-owls/redigit/internal/infra/profiling"
	"github.com/wrapped-owls/redigit/internal/transport/cli/session"
)

func TestRegisterRunFlags(t *testing.T) {
	t.Parallel()

	set := newFlagSet()
	opts := registerRunFlags(set)

	args := []string{
		"--cpuprofile",
		"cpu.out",
		"--memprofile=mem.out",
		"--trace",
		"t.out",
		"--mem-limit",
		"1GiB",
	}
	if err := set.Parse(args); err != nil {
		t.Fatalf("Parse() err = %v", err)
	}

	want := runOptions{
		cpuProfile: "cpu.out", memProfile: "mem.out", tracePath: "t.out", memLimit: "1GiB",
	}
	if *opts != want {
		t.Fatalf("options = %+v, want %+v", *opts, want)
	}
}

func TestDispatchProfiles(t *testing.T) {
	original := debug.SetMemoryLimit(-1)

	t.Cleanup(func() { debug.SetMemoryLimit(original) })

	dir := t.TempDir()
	cpu := filepath.Join(dir, "cpu.pprof")
	mem := filepath.Join(dir, "mem.pprof")
	runTrace := filepath.Join(dir, "run.trace")

	start, _ := settledStart(gomock.NewController(t), nil)

	var stdout, stderr bytes.Buffer

	args := []string{
		cmdGraph, "--repo", ".", "--cpuprofile", cpu, "--memprofile", mem,
		"--trace", runTrace, "--mem-limit", "1GiB",
	}
	if err := dispatch(t.Context(), args, &stdout, &stderr, runEnv{Start: start}); err != nil {
		t.Fatalf("dispatch() err = %v", err)
	}

	for _, path := range []string{cpu, mem} {
		if content, err := os.ReadFile(
			path,
		); err != nil ||
			!bytes.HasPrefix(content, []byte{0x1f, 0x8b}) {
			t.Fatalf("%s = %x, %v, want a gzip profile", path, content[:min(2, len(content))], err)
		}
	}

	if content, err := os.ReadFile(
		runTrace,
	); err != nil ||
		!bytes.HasPrefix(content, []byte("go 1.")) {
		t.Fatalf("trace file = %q, %v, want a trace header", content, err)
	}

	if got := debug.SetMemoryLimit(-1); got != original {
		t.Fatalf("memory limit = %d after the run, want %d restored", got, original)
	}

	if stdout.Len() == 0 {
		t.Fatal("the command wrote no output")
	}
}

func TestDispatchProfilingFailures(t *testing.T) {
	testCases := []struct {
		name      string
		args      []string
		wantUsage bool
		wantCause error
	}{
		{
			"a bad memory limit",
			[]string{cmdGraph, "--repo", ".", "--mem-limit", "lots"},
			true,
			profiling.ErrMemLimit,
		},
		{
			"a cpu profile and a trace in one file",
			[]string{
				cmdGraph, "--repo", ".", "--cpuprofile", "{dir}/x.out", "--trace", "{dir}/./x.out",
			},
			true,
			profiling.ErrSamePath,
		},
		{
			"a cpu and a heap profile in one file",
			[]string{cmdGraph, "--repo", ".", "--cpuprofile", "{dir}/p", "--memprofile", "{dir}/p"},
			true,
			profiling.ErrSamePath,
		},
		{
			"an unwritable cpu profile",
			[]string{cmdGraph, "--repo", ".", "--cpuprofile", "/nonexistent-dir/cpu.pprof"},
			false,
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			dir, args := t.TempDir(), slices.Clone(testCase.args)
			for idx := range args {
				args[idx] = strings.ReplaceAll(args[idx], "{dir}", dir)
			}

			err := dispatch(
				t.Context(),
				args,
				&stdout,
				&stderr,
				runEnv{Start: unreachableStart(t)},
			)
			if err == nil {
				t.Fatal("dispatch() err = nil, want a failure")
			}

			if got := errors.Is(err, ErrUsage); got != testCase.wantUsage {
				t.Fatalf(
					"errors.Is(err, ErrUsage) = %v, want %v (err = %v)",
					got,
					testCase.wantUsage,
					err,
				)
			}

			if !testCase.wantUsage && !strings.HasPrefix(err.Error(), "start profiling:") {
				t.Fatalf("err = %v, want it wrapped as a profiling start failure", err)
			}

			if testCase.wantUsage && !strings.Contains(stderr.String(), "--cpuprofile") {
				t.Fatalf("stderr = %q, want the run flags usage", stderr.String())
			}

			if testCase.wantUsage && !errors.Is(err, testCase.wantCause) {
				t.Fatalf("err = %v, want it to keep the limit's cause", err)
			}
		})
	}
}

func unreachableStart(t *testing.T) starter {
	t.Helper()

	return func(context.Context, bootstrap.Config) (pipeline, error) {
		t.Fatal("a failed profiling start must not start a pipeline")

		return nil, nil
	}
}

func TestDispatchTimings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		args     []string
		graphErr error
		want     string
	}{
		{
			"a run reports each stage it spent time in",
			[]string{cmdGraph, "--repo", ".", "--timings"},
			nil,
			"{\"timings\":{\"close\":0,\"graph\":2000,\"order\":500,\"start\":100,\"total\":2600}}\n",
		},
		{
			"a failed run still reports",
			[]string{cmdGraph, "--repo", ".", "--timings"},
			errors.New("index broke"),
			"{\"timings\":{\"close\":0,\"graph\":2000,\"start\":100,\"total\":2100}}\n",
		},
		{"no flag no line", []string{cmdGraph, "--repo", "."}, nil, ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				pl := NewMockPipeline(gomock.NewController(t))
				pl.EXPECT().Unseen().Return(nil).AnyTimes()
				pl.EXPECT().Unclaimed().Return(nil).AnyTimes()
				pl.EXPECT().Close(gomock.Any()).Return(nil)
				pl.EXPECT().Graph(gomock.Any()).DoAndReturn(
					func(context.Context) (indexing.Result, error) {
						synctest.Sleep(2 * time.Second)

						return indexing.Result{Graph: declGraph()}, testCase.graphErr
					})

				if testCase.graphErr == nil {
					pl.EXPECT().Order(gomock.Any()).DoAndReturn(
						func(entities.Graph) (entities.Ordering, error) {
							synctest.Sleep(500 * time.Millisecond)

							return entities.Ordering{
								At:     map[entities.DeclKey]entities.Position{declKey("A"): 0},
								Rounds: 1,
							}, nil
						})
				}

				start := func(context.Context, bootstrap.Config) (pipeline, error) {
					synctest.Sleep(100 * time.Millisecond)

					return pl, nil
				}

				var stdout, stderr bytes.Buffer

				err := dispatch(t.Context(), testCase.args, &stdout, &stderr, runEnv{Start: start})
				if (err != nil) != (testCase.graphErr != nil) {
					t.Fatalf("dispatch() err = %v, want failure %v", err, testCase.graphErr)
				}

				if got := stderr.String(); got != testCase.want {
					t.Fatalf("stderr = %q, want %q", got, testCase.want)
				}
			})
		})
	}
}

func cacheTestEnv(dir string, start starter) runEnv {
	return runEnv{
		Start:    start,
		CacheDir: dir,
		BuildID:  func() (string, error) { return "build-1", nil },
		Commit:   func(context.Context, string, string) (string, error) { return "commit-1", nil },
		Digest:   func(command string) (string, error) { return "digest-of-" + command, nil },
		Toolchain: func(_ context.Context, repo, commit string) (string, error) {
			return "toolchain-of-" + repo + "@" + commit, nil
		},
	}
}

func parsedSet(t *testing.T, args ...string) (*flag.FlagSet, bootstrap.Config) {
	t.Helper()

	set := newFlagSet()
	registerRunFlags(set)

	cfg, _, err := parse(set, args)
	if err != nil {
		t.Fatalf("parse(%v) err = %v", args, err)
	}

	return set, cfg
}

func TestEveryFlagIsClassified(t *testing.T) {
	t.Parallel()

	set, _ := parsedSet(t, "--repo", ".")

	names := 0

	set.VisitAll(func(setFlag *flag.Flag) {
		names++

		if _, isKnown := session.Classify(setFlag.Name); !isKnown {
			t.Errorf(
				"flag --%s is not classified: name it a key, file or run-only flag",
				setFlag.Name,
			)
		}
	})

	if names < 12 {
		t.Fatalf("only %d flags registered, want the full set", names)
	}
}

func TestDispatchStillWritesTheProfileWhenTheRunFails(t *testing.T) {
	errGraph := errors.New("resolver unreachable")
	cpu := filepath.Join(t.TempDir(), "cpu.pprof")
	start, _ := settledStart(gomock.NewController(t), errGraph)

	var stdout, stderr bytes.Buffer

	args := []string{cmdGraph, "--repo", ".", "--cpuprofile", cpu}

	err := dispatch(t.Context(), args, &stdout, &stderr, runEnv{Start: start})
	if !errors.Is(err, errGraph) {
		t.Fatalf("dispatch() err = %v, want the graph failure", err)
	}

	content, readErr := os.ReadFile(cpu)
	if readErr != nil || len(content) == 0 {
		t.Fatalf("profile = %d bytes, %v, want a profile after a failed run", len(content), readErr)
	}

	reader, gzipErr := gzip.NewReader(bytes.NewReader(content))
	if gzipErr != nil {
		t.Fatalf("profile is not gzip: %v", gzipErr)
	}

	if body, copyErr := io.ReadAll(reader); copyErr != nil || len(body) == 0 {
		t.Fatalf("profile body = %d bytes, %v, want a readable profile", len(body), copyErr)
	}
}

func TestDispatchTimingsShowTheCostOfTheKey(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		counter := &startCounter{live: settledLive(t)}
		env := cacheTestEnv(t.TempDir(), counter.start)
		env.Toolchain = func(context.Context, string, string) (string, error) {
			synctest.Sleep(300 * time.Millisecond)

			return "toolchain", nil
		}

		testCases := []struct {
			name     string
			wantSave bool
		}{
			{"a miss pays for the save", true},
			{"a hit pays for none", false},
		}

		for _, testCase := range testCases {
			var stderr bytes.Buffer

			args := []string{cmdGraph, "--repo", ".", "--from-cache", "--timings"}
			if err := dispatch(t.Context(), args, io.Discard, &stderr, env); err != nil {
				t.Fatalf("%s: dispatch() err = %v", testCase.name, err)
			}

			var line struct {
				Timings map[string]float64 `json:"timings"`
			}

			if err := json.Unmarshal(stderr.Bytes(), &line); err != nil {
				t.Fatalf(
					"%s: stderr = %q, want one JSON line: %v",
					testCase.name,
					stderr.String(),
					err,
				)
			}

			if got := line.Timings["key"]; got != 300 {
				t.Fatalf("%s: key stage = %v ms in %v, want the 300 the probe took",
					testCase.name, got, line.Timings)
			}

			if _, isSaved := line.Timings["save"]; isSaved != testCase.wantSave {
				t.Fatalf("%s: timings %v, want a save stage = %v",
					testCase.name, line.Timings, testCase.wantSave)
			}
		}
	})
}

func TestRealEnvIsWiredToTheRealCollaborators(t *testing.T) {
	t.Parallel()

	env := realEnv()
	ctx := t.Context()

	testCases := []struct {
		name string
		got  func() (string, error)
		want func() (string, error)
	}{
		{"build id", env.BuildID, plancache.ExecutableDigest},
		{
			"commit",
			func() (string, error) { return env.Commit(ctx, ".", "HEAD") },
			func() (string, error) { return gitstore.ResolveCommit(ctx, ".", "HEAD") },
		},
		{
			"command digest",
			func() (string, error) { return env.Digest("git") },
			func() (string, error) { return plancache.CommandDigest("git") },
		},
	}

	for _, testCase := range testCases {
		got, gotErr := testCase.got()
		want, wantErr := testCase.want()

		if gotErr != nil || wantErr != nil || got == "" || got != want {
			t.Errorf("%s = %q, %v, want %q, %v", testCase.name, got, gotErr, want, wantErr)
		}
	}

	if _, err := env.Toolchain(ctx, t.TempDir(), "HEAD"); !errors.Is(err, plancache.ErrToolchain) {
		t.Errorf("Toolchain() err = %v, want the plan cache's ErrToolchain", err)
	}

	_, err := env.Start(ctx, bootstrap.Config{Repo: t.TempDir()})
	if err == nil || !strings.HasPrefix(err.Error(), "bootstrap start:") {
		t.Errorf("Start() err = %v, want the bootstrap start failure", err)
	}
}

func TestFinishRunReportsEveryFailureItMet(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("no /dev/full to refuse the heap profile")
	}

	profile, err := profiling.Start(profiling.Options{MemProfile: "/dev/full"})
	if err != nil {
		t.Fatalf("Start() err = %v", err)
	}

	errRun, errWrite := errors.New("run failed"), errors.New("stderr closed")
	opts := &runOptions{isTiming: true}

	err = finishRun(profile, profiling.NewTimings(), opts, failWriter{err: errWrite}, errRun)

	if !errors.Is(err, errRun) || !errors.Is(err, errWrite) ||
		!strings.Contains(err.Error(), "stop profiling:") {
		t.Fatalf("finishRun() err = %v, want the run, the timings write and the profile stop", err)
	}
}
