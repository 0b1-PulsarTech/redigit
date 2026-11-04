package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
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

func TestDispatchWriteRoutesWithoutAPipeline(t *testing.T) {
	t.Parallel()

	unreachable := func(context.Context, bootstrap.Config) (pipeline, error) {
		t.Fatal("write must never start a pipeline")

		return nil, nil
	}

	identity := []string{"--author-name", "n", "--author-email", "e", "--date-ref", "HEAD"}

	testCases := []struct {
		name        string
		args        []string
		wantUsage   bool
		wantMessage string
	}{
		{"missing flags print the usage", []string{cmdWrite}, true, ""},
		{"unknown flag prints the usage", []string{cmdWrite, "--nope"}, true, ""},
		{
			"a run failure prints no usage",
			append(
				[]string{
					cmdWrite,
					"--repo",
					".",
					"--ref",
					"refs/heads/x",
					"--manifest",
					"/nonexistent",
				},
				identity...),
			false,
			"open manifest",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			err := dispatch(
				context.Background(), testCase.args, &stdout, &stderr, runEnv{Start: unreachable},
			)
			if err == nil || errors.Is(err, ErrUsage) != testCase.wantUsage {
				t.Fatalf("err = %v, want usage = %v", err, testCase.wantUsage)
			}

			if got := strings.Contains(
				stderr.String(),
				"redigit write",
			); got != testCase.wantUsage {
				t.Fatalf("stderr = %q, want write usage = %v", stderr.String(), testCase.wantUsage)
			}

			if !strings.Contains(err.Error(), testCase.wantMessage) || stdout.Len() != 0 {
				t.Fatalf("err = %v, stdout = %q", err, stdout.String())
			}
		})
	}
}

func TestDispatchVerifyRoutesWithoutAPipeline(t *testing.T) {
	t.Parallel()

	unreachable := func(context.Context, bootstrap.Config) (pipeline, error) {
		t.Fatal("verify must never start a pipeline")

		return nil, nil
	}

	testCases := []struct {
		name        string
		args        []string
		wantUsage   bool
		wantMessage string
	}{
		{"unknown flag prints the usage", []string{cmdVerify, "--nope"}, true, ""},
		{"zero stride prints the usage", []string{cmdVerify, "--stride", "0"}, true, ""},
		{
			"a run failure prints no usage",
			[]string{cmdVerify, "--repo", "/nonexistent/redigit/repo"},
			false,
			"find git dir",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			err := dispatch(
				context.Background(), testCase.args, &stdout, &stderr, runEnv{Start: unreachable},
			)
			if err == nil || errors.Is(err, ErrUsage) != testCase.wantUsage {
				t.Fatalf("err = %v, want usage = %v", err, testCase.wantUsage)
			}

			if got := strings.Contains(
				stderr.String(),
				"redigit verify",
			); got != testCase.wantUsage {
				t.Fatalf("stderr = %q, want verify usage = %v", stderr.String(), testCase.wantUsage)
			}

			if !strings.Contains(err.Error(), testCase.wantMessage) || stdout.Len() != 0 {
				t.Fatalf("err = %v, stdout = %q", err, stdout.String())
			}
		})
	}
}

func TestDispatchReservedVerbsAreUsageErrors(t *testing.T) {
	t.Parallel()

	unreachable := func(context.Context, bootstrap.Config) (pipeline, error) {
		t.Fatal("a reserved verb must never start a pipeline")

		return nil, nil
	}

	testCases := []struct {
		name    string
		command string
		slice   string
	}{
		{"format", cmdFormat, "slice 15w"},
		{"continue", cmdContinue, "slice 24b"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			err := dispatch(
				context.Background(),
				[]string{testCase.command, "--repo", "."},
				&stdout,
				&stderr,
				runEnv{Start: unreachable},
			)
			if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), testCase.slice) {
				t.Fatalf("err = %v, want a usage error naming %q", err, testCase.slice)
			}

			if stderr.String() != fullUsage || stdout.Len() != 0 {
				t.Fatalf(
					"stderr = %q, stdout = %q, want the full usage alone",
					stderr.String(),
					stdout.String(),
				)
			}
		})
	}
}

func TestDispatchLintRunsItsOwnCommandWithoutAPipeline(t *testing.T) {
	t.Parallel()

	unreachable := func(context.Context, bootstrap.Config) (pipeline, error) {
		t.Fatal("lint must never start a pipeline")

		return nil, nil
	}

	testCases := []struct {
		name        string
		args        []string
		wantUsage   bool
		wantMessage string
	}{
		{
			name: "no manifest is lint's own usage error", args: []string{cmdLint},
			wantUsage: true, wantMessage: "--manifest is required",
		},
		{
			name:        "a missing manifest file is a run failure, not usage",
			args:        []string{cmdLint, "--manifest", "/nonexistent/m.jsonl"},
			wantMessage: "open manifest",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			err := dispatch(
				context.Background(), testCase.args, &stdout, &stderr, runEnv{Start: unreachable},
			)
			if err == nil || errors.Is(err, ErrUsage) != testCase.wantUsage ||
				!strings.Contains(err.Error(), testCase.wantMessage) || stdout.Len() != 0 {
				t.Fatalf("err = %v, stdout = %q", err, stdout.String())
			}

			if got := strings.Contains(stderr.String(), "redigit lint"); got != testCase.wantUsage {
				t.Fatalf("stderr = %q, want lint usage = %v", stderr.String(), testCase.wantUsage)
			}
		})
	}
}
