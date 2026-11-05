package resume

import (
	"context"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type walkStep struct {
	args        []string
	result      error
	isEdited    bool
	isRefused   bool
	wantErr     error
	wantLeft    []string
	wantDecided map[string]anchoring.Policy
	wantAnchors map[string]anchoring.Policy
}

type walkCase struct {
	name  string
	steps []walkStep
}

func policies(policy anchoring.Policy, revs ...string) map[string]anchoring.Policy {
	out := map[string]anchoring.Policy{}
	for _, rev := range revs {
		out[rev] = policy
	}

	return out
}

func foldArg(rev string) []string { return []string{"--state", "{dir}", "--fold", rev} }

func TestContinueWalks(t *testing.T) {
	t.Parallel()

	var (
		boom      = errors.New("boom")
		all       = []string{firstRev, secondRev, thirdRev}
		rest      = []string{secondRev, thirdRev}
		last      = []string{thirdRev}
		stopped   = anchoring.ErrStopped
		foldFirst = policies(anchoring.PolicyFold, firstRev)
		foldTwo   = policies(anchoring.PolicyFold, firstRev, secondRev)
		anchFirst = policies(anchoring.PolicyFold, "3d2d883")
		anchTwo   = policies(anchoring.PolicyFold, "3d2d883", "0aab4c6")
	)

	checkWalks(t, []walkCase{
		{name: "a stop of continue keeps the decision and the conflicts left", steps: []walkStep{{
			args: foldArg("3d2d883"), result: stoppedAt("Run", rest...), wantErr: stopped,
			wantLeft: rest, wantDecided: foldFirst, wantAnchors: anchFirst,
		}}},
		{
			name: "a failure at depth one leaves the state and the same command goes on",
			steps: []walkStep{
				{
					args: foldArg("3d2d883"), result: boom, wantErr: boom, wantLeft: all,
					wantAnchors: anchFirst,
				},
				{
					args: foldArg("3d2d883"), result: stoppedAt("Run", rest...), wantErr: stopped,
					wantLeft: rest, wantDecided: foldFirst, wantAnchors: anchFirst,
				},
			},
		},
		{name: "a failure at depth two leaves the first decision and goes on", steps: []walkStep{
			{
				args: foldArg("3d2d883"), result: stoppedAt("Run", rest...), wantErr: stopped,
				wantLeft: rest, wantDecided: foldFirst, wantAnchors: anchFirst,
			},
			{
				args: foldArg("0aab4c6"), result: boom, wantErr: boom, wantLeft: rest,
				wantDecided: foldFirst, wantAnchors: anchTwo,
			},
			{
				args: foldArg("0aab4c6"), result: stoppedAt("Run", last...), wantErr: stopped,
				wantLeft: last, wantDecided: foldTwo, wantAnchors: anchTwo,
			},
		}},
	})
}

func TestContinueWalksCarryEveryDecision(t *testing.T) {
	t.Parallel()

	var (
		boom     = errors.New("boom")
		all      = []string{firstRev, secondRev, thirdRev}
		rest     = []string{secondRev, thirdRev}
		last     = []string{thirdRev}
		stopped  = anchoring.ErrStopped
		toAll    = []string{"--state", "{dir}", "--rest", "fold"}
		mixedTwo = map[string]anchoring.Policy{
			firstRev:  anchoring.PolicyWIP,
			secondRev: anchoring.PolicyFold,
		}
		anchMixed = map[string]anchoring.Policy{
			"3d2d883": anchoring.PolicyWIP, "0aab4c6": anchoring.PolicyFold,
		}
	)

	checkWalks(t, []walkCase{
		{name: "a rerun after a finish goes the same way", steps: []walkStep{
			{args: toAll, wantLeft: all},
			{args: toAll, wantLeft: all},
		}},
		{name: "a failure at depth three and a finish carry every decision", steps: []walkStep{
			{
				args: []string{"--state", "{dir}", "--wip", "3d2d883"}, wantErr: stopped,
				result:   stoppedAt("Run", rest...),
				wantLeft: rest, wantDecided: policies(anchoring.PolicyWIP, firstRev),
				wantAnchors: policies(anchoring.PolicyWIP, "3d2d883"),
			},
			{
				args: foldArg("0aab4c6"), result: stoppedAt("Run", last...), wantErr: stopped,
				wantLeft: last, wantDecided: mixedTwo, wantAnchors: anchMixed,
			},
			{
				args: toAll, result: boom, wantErr: boom, wantLeft: last,
				wantDecided: mixedTwo, wantAnchors: anchMixed,
			},
			{args: toAll, wantLeft: last, wantDecided: mixedTwo, wantAnchors: anchMixed},
		}},
	})
}

func TestContinueWalksRefuse(t *testing.T) {
	t.Parallel()

	var (
		all       = []string{firstRev, secondRev, thirdRev}
		rest      = []string{secondRev, thirdRev}
		last      = []string{thirdRev}
		stopped   = anchoring.ErrStopped
		toAll     = []string{"--state", "{dir}", "--rest", "fold"}
		foldFirst = policies(anchoring.PolicyFold, firstRev)
		foldTwo   = policies(anchoring.PolicyFold, firstRev, secondRev)
		anchFirst = policies(anchoring.PolicyFold, "3d2d883")
		anchTwo   = policies(anchoring.PolicyFold, "3d2d883", "0aab4c6")
	)

	checkWalks(t, []walkCase{
		{name: "a config edited before depth one is stale", steps: []walkStep{
			{
				args:      toAll,
				isEdited:  true,
				isRefused: true,
				wantErr:   anchorstate.ErrStale,
				wantLeft:  all,
			},
		}},
		{name: "a config edited at depth two is stale", steps: []walkStep{
			{
				args: foldArg("3d2d883"), result: stoppedAt("Run", rest...), wantErr: stopped,
				wantLeft: rest, wantDecided: foldFirst, wantAnchors: anchFirst,
			},
			{
				args: foldArg("0aab4c6"), isEdited: true, isRefused: true,
				wantErr: anchorstate.ErrStale, wantLeft: rest, wantDecided: foldFirst,
			},
		}},
		{name: "a config edited at depth three is stale", steps: []walkStep{
			{
				args: foldArg("3d2d883"), result: stoppedAt("Run", rest...), wantErr: stopped,
				wantLeft: rest, wantDecided: foldFirst, wantAnchors: anchFirst,
			},
			{
				args: foldArg("0aab4c6"), result: stoppedAt("Run", last...), wantErr: stopped,
				wantLeft: last, wantDecided: foldTwo, wantAnchors: anchTwo,
			},
			{
				args: toAll, isEdited: true, isRefused: true, wantErr: anchorstate.ErrStale,
				wantLeft: last, wantDecided: foldTwo,
			},
		}},
		{name: "an anchor already decided cannot be decided again", steps: []walkStep{
			{
				args: foldArg("3d2d883"), result: stoppedAt("Run", rest...), wantErr: stopped,
				wantLeft: rest, wantDecided: foldFirst, wantAnchors: anchFirst,
			},
			{
				args: foldArg("3d2d883"), isRefused: true, wantErr: write.ErrUsage,
				wantLeft: rest, wantDecided: foldFirst,
			},
		}},
	})
}

func checkWalks(t *testing.T, testCases []walkCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runWalk(t, testCase.steps)
		})
	}
}

func runWalk(t *testing.T, steps []walkStep) {
	t.Helper()

	const walkConfig = `{"lineage": {"policy": "stop", "anchors": [
		{"rev": "3d2d883", "kind": "fix"}, {"rev": "0aab4c6", "kind": "fix"},
		{"rev": "9f8e7d6", "kind": "fix"}]}}`

	dir, config := savedState(t, walkConfig, firstRev, secondRev, thirdRev)
	fake := &recorder{}
	run := continuer{
		dispatch: fake.dispatch,
		flags:    testFlags,
		commit:   func(context.Context, string, string) (string, error) { return "c1", nil },
	}

	for idx, step := range steps {
		if step.isEdited {
			if err := os.WriteFile(
				config,
				[]byte(`{"lineage": {"policy": "stop"}}`),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
		}

		runs := len(fake.argv)
		fake.err = step.result

		err := run.run(context.Background(), inDir(step.args, dir), io.Discard)
		if !errors.Is(err, step.wantErr) {
			t.Fatalf("step %d: run() = %v, want %v", idx, err, step.wantErr)
		}

		if got := len(fake.argv) - runs; (got == 0) != step.isRefused || got > 1 {
			t.Fatalf("step %d: dispatched %d times, refused = %v", idx, got, step.isRefused)
		}

		checkWalkState(t, idx, dir, step)
	}
}

func checkWalkState(t *testing.T, idx int, dir string, step walkStep) {
	t.Helper()

	state, err := anchorstate.Load(dir)
	if err != nil {
		t.Fatalf("step %d: Load() = %v", idx, err)
	}

	left := make([]string, 0, len(state.Conflicts))

	for _, conflict := range state.Conflicts {
		left = append(left, conflict.Rev)
	}

	if !slices.Equal(left, step.wantLeft) || !maps.Equal(state.Decisions, step.wantDecided) {
		t.Fatalf("step %d: state left %v decided %v, want %v %v",
			idx, left, state.Decisions, step.wantLeft, step.wantDecided)
	}

	if step.isRefused {
		return
	}

	got := map[string]anchoring.Policy{}

	for _, spec := range derivedSection(t, dir).Anchors {
		if spec.Policy != "" {
			got[spec.Rev] = spec.Policy
		}
	}

	if !maps.Equal(got, step.wantAnchors) {
		t.Fatalf("step %d: derived anchors %v, want %v", idx, got, step.wantAnchors)
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		raw      string
		saved    *anchorstate.State
		wantErr  error
		wantNot  error
		wantText string
	}{
		{name: "no state is a usage error", wantErr: write.ErrUsage, wantText: "no stop state"},
		{name: "an empty object is not ours", raw: `{}`, wantErr: write.ErrUsage},
		{name: "null is not ours", raw: `null`, wantErr: write.ErrUsage},
		{name: "an empty command is not ours", raw: `{"args":[]}`, wantErr: write.ErrUsage},
		{name: "text that is not json is not ours", raw: `{`, wantErr: write.ErrUsage},
		{
			name: "our state with no command is corrupt", saved: &anchorstate.State{},
			wantErr: anchorstate.ErrCorrupt, wantNot: write.ErrUsage,
		},
		{
			name: "our state that runs continue again is corrupt", wantNot: write.ErrUsage,
			saved:   &anchorstate.State{Args: []string{Verb, "--state", "x"}},
			wantErr: anchorstate.ErrCorrupt, wantText: "continue",
		},
		{name: "our state is loaded", saved: &anchorstate.State{Args: []string{"emit"}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")

			if testCase.raw != "" {
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(
					filepath.Join(dir, "state.json"),
					[]byte(testCase.raw),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			}

			if testCase.saved != nil {
				if err := anchorstate.Save(dir, *testCase.saved); err != nil {
					t.Fatal(err)
				}
			}

			_, err := load(dir)
			if !errors.Is(err, testCase.wantErr) ||
				(testCase.wantNot != nil && errors.Is(err, testCase.wantNot)) ||
				(err != nil && !strings.Contains(err.Error(), testCase.wantText)) {
				t.Fatalf("load() = %v, want %v, not %v, text %q",
					err, testCase.wantErr, testCase.wantNot, testCase.wantText)
			}
		})
	}
}

func TestDerive(t *testing.T) {
	t.Parallel()

	head := `{"pad":"","lineage":{"policy":"stop","anchors":[{"rev":"3d2d883","kind":"fix"}]}}`
	pad := strings.Repeat("x", 1<<20-len(head))
	oversized := strings.Replace(head, `"pad":""`, `"pad":"`+pad+`"`, 1)

	testCases := []struct {
		name    string
		content string
		isGone  bool
		wantErr error
		isFail  bool
	}{
		{name: "a config with the decisions in it", content: stopConfig},
		{name: "a config that is gone", isGone: true, wantErr: os.ErrNotExist},
		{name: "a config that is not json", content: `{`, isFail: true},
		{name: "a derived config over the limit", content: oversized, wantErr: rules.ErrTooLarge},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := filepath.Join(t.TempDir(), "cfg.json")

			if !testCase.isGone {
				if err := os.WriteFile(config, []byte(testCase.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			_, err := derive(
				config,
				map[string]anchoring.Policy{firstRev: anchoring.PolicyFold},
				"",
			)

			isRefused := testCase.wantErr != nil || testCase.isFail
			if (err != nil) != isRefused ||
				(testCase.wantErr != nil && !errors.Is(err, testCase.wantErr)) {
				t.Fatalf("derive() = %v, want %v (fail %v)", err, testCase.wantErr, testCase.isFail)
			}
		})
	}
}

func TestVerify(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	testCases := []struct {
		name      string
		commit    string
		commitErr error
		isOwn     bool
		wantErr   error
	}{
		{name: "a state still fresh", commit: "c1"},
		{name: "a moved revision is stale", commit: "c2", wantErr: anchorstate.ErrStale},
		{name: "a revision that cannot be resolved", commitErr: boom, wantErr: boom},
		{
			name:    "a config that is the derived file",
			commit:  "c1",
			isOwn:   true,
			wantErr: write.ErrUsage,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir, config := stoppedState(t)

			state, err := anchorstate.Load(dir)
			if err != nil {
				t.Fatal(err)
			}

			if testCase.isOwn {
				config = filepath.Join(dir, anchorstate.DerivedFile)
			}

			run := continuer{
				flags: testFlags,
				commit: func(context.Context, string, string) (string, error) {
					return testCase.commit, testCase.commitErr
				},
			}

			err = run.verify(context.Background(), state, target{config: config}, dir)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("verify() = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
