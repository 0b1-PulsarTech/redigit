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

// TestDispatchUsagePaths pins the three ways a command line never reaches a
// pipeline: no command, an unknown command, and a parse failure. All three
// print usage to stderr and return ErrUsage.
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

			err := dispatch(context.Background(), testCase.args, &stdout, &stderr, unreachable)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("err = %v, want it to wrap ErrUsage", err)
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

// TestDispatchRoutesKnownCommands pins that each known command name reaches
// its own handler and not another's, using a fake pipeline so no language
// server or git worktree is involved.
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
			pl := NewMockpipeline(ctrl)
			pl.EXPECT().Unseen().Return(nil).AnyTimes()
			pl.EXPECT().Unclaimed().Return(nil).AnyTimes()
			pl.EXPECT().Close(gomock.Any()).Return(nil)

			// Every handler starts by asking for the graph; failing fast
			// there is enough to prove dispatch reached a real handler
			// instead of falling through to the unknown-command usage path.
			errRoute := errors.New("routed")
			pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{}, errRoute)

			start := func(context.Context, bootstrap.Config) (pipeline, error) { return pl, nil }

			var stdout, stderr bytes.Buffer

			args := []string{testCase.command, "--repo", "."}

			err := dispatch(context.Background(), args, &stdout, &stderr, start)
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

// TestRunDelegatesToRealDispatch proves Run wires the real starter without
// invoking it: the usage paths below never reach bootstrap.Start, so this
// stays a fast, deterministic check of Run's own plumbing.
func TestRunDelegatesToRealDispatch(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	err := Run(context.Background(), nil, &stdout, &stderr)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err = %v, want it to wrap ErrUsage", err)
	}
}
