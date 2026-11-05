package resume

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type continueCase struct {
	name       string
	args       []string
	commit     string
	commitErr  error
	isEdited   bool
	wantErr    error
	wantRuns   int
	wantPolicy anchoring.Policy
	wantSpecs  []anchoring.Spec
}

func TestContinue(t *testing.T) {
	t.Parallel()

	both := []anchoring.Spec{{Rev: "3d2d883", Kind: "fix"}, {Rev: "0aab4c6", Kind: "perf"}}

	testCases := []continueCase{
		{
			name:       "rest fold re-dispatches the original argv with the derived config",
			args:       []string{"--state", "{dir}", "--rest", "fold"},
			commit:     "c1",
			wantRuns:   1,
			wantPolicy: anchoring.PolicyFold,
			wantSpecs:  both,
		},
		{
			name:       "rest wip",
			args:       []string{"--state", "{dir}", "--rest", "wip"},
			commit:     "c1",
			wantRuns:   1,
			wantPolicy: anchoring.PolicyWIP,
			wantSpecs:  both,
		},
		{
			name: "fold one conflict and leave the rest to stop again",
			args: []string{"--state", "{dir}", "--fold", "3d2d883"}, commit: "c1", wantRuns: 1,
			wantPolicy: anchoring.PolicyStop,
			wantSpecs: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix", Policy: anchoring.PolicyFold},
				{Rev: "0aab4c6", Kind: "perf"},
			},
		},
		{
			name: "wip one conflict and rest fold the others",
			args: []string{"--state", "{dir}", "--wip", "0aab4c6", "--rest", "fold"}, commit: "c1",
			wantRuns: 1, wantPolicy: anchoring.PolicyFold,
			wantSpecs: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix"},
				{Rev: "0aab4c6", Kind: "perf", Policy: anchoring.PolicyWIP},
			},
		},
	}

	checkContinue(t, testCases)
}

func checkContinue(t *testing.T, testCases []continueCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runContinue(t, testCase)
		})
	}
}

func TestContinueRefusals(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	rest := []string{"--state", "{dir}", "--rest", "fold"}

	testCases := []continueCase{
		{
			name: "an unknown rev is a usage error", commit: "c1", wantErr: write.ErrUsage,
			args: []string{"--state", "{dir}", "--fold", "beef000"},
		},
		{
			name:    "a rev given two decisions is a usage error",
			commit:  "c1",
			wantErr: write.ErrUsage,
			args:    []string{"--state", "{dir}", "--fold", "3d2d883", "--wip", "3d2d883"},
		},
		{
			name: "an unknown rest is a usage error", commit: "c1", wantErr: write.ErrUsage,
			args: []string{"--state", "{dir}", "--rest", "later"},
		},
		{
			name: "nothing to decide is a usage error", commit: "c1", wantErr: write.ErrUsage,
			args: []string{"--state", "{dir}"},
		},
		{
			name:    "no state flag is a usage error",
			wantErr: write.ErrUsage,
			args:    []string{"--rest", "fold"},
		},
		{
			name: "a directory with no state is a usage error", wantErr: write.ErrUsage,
			args: []string{"--state", "{dir}/missing", "--rest", "fold"},
		},
		{
			name:    "a moved revision is stale",
			commit:  "c2",
			wantErr: anchorstate.ErrStale,
			args:    rest,
		},
		{
			name: "an edited config is stale", commit: "c1", wantErr: anchorstate.ErrStale,
			args: rest, isEdited: true,
		},
		{
			name: "a revision that cannot be resolved is a failure", commitErr: boom,
			wantErr: boom, args: rest,
		},
		{
			name: "an unknown flag is a usage error", commit: "c1", wantErr: write.ErrUsage,
			args: []string{"--state", "{dir}", "--rest", "fold", "--nope"},
		},
	}

	checkContinue(t, testCases)
}

func runContinue(t *testing.T, testCase continueCase) {
	t.Helper()

	dir, config := stoppedState(t)
	fake, asked := &recorder{}, [2]string{}
	run := continuer{
		dispatch: fake.dispatch,
		flags:    testFlags,
		commit: func(_ context.Context, repo, rev string) (string, error) {
			asked = [2]string{repo, rev}

			return testCase.commit, testCase.commitErr
		},
	}

	if testCase.isEdited {
		if err := os.WriteFile(
			config,
			[]byte(`{"lineage": {"policy": "stop"}}`),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer

	err := run.run(context.Background(), inDir(testCase.args, dir), &stdout)
	if !errors.Is(err, testCase.wantErr) {
		t.Fatalf("run() = %v, want %v", err, testCase.wantErr)
	}

	if len(fake.argv) != testCase.wantRuns {
		t.Fatalf("dispatched %d times, want %d", len(fake.argv), testCase.wantRuns)
	}

	if testCase.wantRuns == 0 {
		return
	}

	if asked != [2]string{"R", "HEAD"} {
		t.Fatalf("the revision was asked as %q, want the repo R at HEAD", asked)
	}

	derived := filepath.Join(dir, anchorstate.DerivedFile)
	got, _ := scan(testFlags, fake.argv[0][1:])

	if got.config != derived || fake.argv[0][0] != "emit" {
		t.Fatalf("argv = %q, want emit with --config %s", fake.argv[0], derived)
	}

	section := derivedSection(t, dir)
	if section.Policy != testCase.wantPolicy || !slices.Equal(section.Anchors, testCase.wantSpecs) {
		t.Fatalf(
			"derived section = %+v, want %q %v",
			section,
			testCase.wantPolicy,
			testCase.wantSpecs,
		)
	}

	if stdout.String() != "ran" {
		t.Fatalf("stdout = %q, want what the dispatched run wrote", stdout.String())
	}
}

func TestContinueWhenTheDerivedConfigCannotBeWritten(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir, _ := stoppedState(t)
	fake := &recorder{}
	run := continuer{
		dispatch: fake.dispatch,
		flags:    testFlags,
		commit:   func(context.Context, string, string) (string, error) { return "c1", nil },
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}

	err := run.run(
		context.Background(),
		inDir([]string{"--state", "{dir}", "--rest", "fold"}, dir),
		io.Discard,
	)
	if err == nil || len(fake.argv) != 0 {
		t.Fatalf("run() = %v after %d dispatches, want a refusal before any", err, len(fake.argv))
	}
}

func TestContinueRefusesAForeignConflictsBeforeRunning(t *testing.T) {
	t.Parallel()

	spoilers := []struct {
		name  string
		spoil func(t *testing.T, dir string)
	}{
		{name: "a conflicts directory that lost its marker", spoil: func(t *testing.T, dir string) {
			t.Helper()

			if err := os.Remove(filepath.Join(dir, "conflicts", ".redigit-conflicts")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a conflicts directory that is a link", spoil: func(t *testing.T, dir string) {
			t.Helper()

			if err := os.RemoveAll(filepath.Join(dir, "conflicts")); err != nil {
				t.Fatal(err)
			}

			if err := os.Symlink(".", filepath.Join(dir, "conflicts")); err != nil {
				t.Fatal(err)
			}
		}},
	}

	decisions := [][]string{
		{"--fold", "3d2d883"}, {"--wip", "3d2d883"}, {"--rest", "fold"}, {"--rest", "wip"},
	}

	for _, spoiler := range spoilers {
		for _, decision := range decisions {
			t.Run(spoiler.name+" "+decision[0]+" "+decision[1], func(t *testing.T) {
				t.Parallel()

				dir, _ := stoppedState(t)
				spoiler.spoil(t, dir)

				fake := &recorder{}
				run := continuer{dispatch: fake.dispatch, flags: testFlags, commit: func(
					context.Context, string, string,
				) (string, error) {
					return "c1", nil
				}}

				args := slices.Concat([]string{"--state", dir}, decision)

				err := run.run(context.Background(), args, io.Discard)
				if !errors.Is(err, write.ErrUsage) || !errors.Is(err, anchorstate.ErrForeign) ||
					len(fake.argv) != 0 {
					t.Fatalf(
						"run() = %v, dispatched %d, want a refusal before any run",
						err,
						len(fake.argv),
					)
				}

				if _, statErr := os.Stat(
					filepath.Join(dir, anchorstate.DerivedFile),
				); statErr == nil {
					t.Fatal("a refused continue wrote the derived config")
				}
			})
		}
	}
}

func TestContinueAbort(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		extra       []string
		files       map[string]string
		isMissing   bool
		wantErr     error
		wantRemoved bool
		wantKept    []string
	}{
		{
			name:        "abort removes the state and nothing else",
			files:       map[string]string{"notes.txt": "mine"},
			wantRemoved: true, wantKept: []string{"notes.txt"},
		},
		{
			name:    "abort with a decision is a usage error",
			extra:   []string{"--rest", "fold"},
			wantErr: write.ErrUsage,
		},
		{
			name: "abort of a state of someone else is a usage error and removes nothing",
			files: map[string]string{
				"state.json": `{"mine":1}`, "conflicts/y": "y",
			},
			wantErr: write.ErrUsage, wantKept: []string{"state.json", "conflicts/y"},
		},
		{
			name: "abort with nothing to abort is a usage error", isMissing: true,
			wantErr: write.ErrUsage, wantRemoved: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir, _ := stoppedState(t)
			if testCase.isMissing {
				dir = filepath.Join(dir, "missing")
			}

			if testCase.files != nil && testCase.wantErr != nil {
				dir = filepath.Join(t.TempDir(), "foreign")
			}

			writeAll(t, dir, testCase.files)

			fake := &recorder{}
			run := continuer{dispatch: fake.dispatch}
			args := slices.Concat([]string{"--state", dir, "--abort"}, testCase.extra)

			err := run.run(context.Background(), args, io.Discard)
			if !errors.Is(err, testCase.wantErr) || len(fake.argv) != 0 {
				t.Fatalf(
					"run() = %v, dispatched %d, want %v and none",
					err,
					len(fake.argv),
					testCase.wantErr,
				)
			}

			if _, loadErr := anchorstate.Load(
				dir,
			); errors.Is(
				loadErr,
				anchorstate.ErrNoState,
			) != testCase.wantRemoved {
				t.Fatalf("state removed = %v, want %v", loadErr, testCase.wantRemoved)
			}

			for _, name := range testCase.wantKept {
				if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
					t.Fatalf("abort removed %s, which is not the state's: %v", name, statErr)
				}
			}
		})
	}
}

func TestWithBuildsTheVerb(t *testing.T) {
	t.Parallel()

	fake := &recorder{}
	verb := With(fake.dispatch, testFlags)

	err := verb(context.Background(), nil, io.Discard)
	if !errors.Is(err, write.ErrUsage) || len(fake.argv) != 0 {
		t.Fatalf("verb() = %v, dispatched %d, want a usage error and none", err, len(fake.argv))
	}
}
