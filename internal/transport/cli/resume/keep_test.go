package resume

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type keepCase struct {
	name        string
	verb        string
	rev         string
	withoutDir  bool
	err         error
	commitErr   error
	isDirAFile  bool
	hasNoConfig bool
	hasForeign  bool
	isSelfFed   bool
	isRevEmpty  bool
	wantIs      []error
	wantNot     []error
	wantText    string
	wantSaved   bool
}

func prepareKeep(t *testing.T, testCase keepCase) (args []string, dir string) {
	t.Helper()

	base := t.TempDir()
	dir = filepath.Join(base, "state")
	config := filepath.Join(base, "cfg.json")

	if !testCase.hasNoConfig {
		if err := os.WriteFile(config, []byte(stopConfig), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if testCase.isDirAFile {
		if err := os.WriteFile(dir, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if testCase.hasForeign {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(
			filepath.Join(dir, "state.json"),
			[]byte(`{"mine":1}`),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}

	if testCase.isSelfFed {
		config = filepath.Join(dir, anchorstate.DerivedFile)
	}

	args = []string{cmp.Or(testCase.verb, "emit"), "--repo", "R", "--config", config}
	if testCase.rev != "" || testCase.isRevEmpty {
		args = append(args, "--rev", testCase.rev)
	}

	if !testCase.withoutDir {
		args = append(args, "--state-dir", dir)
	}

	return args, dir
}

func runKeep(t *testing.T, testCase keepCase) {
	t.Helper()

	var (
		gotRev string
		logged bytes.Buffer
	)

	args, dir := prepareKeep(t, testCase)
	keep := keeper{
		flags: testFlags,
		commit: func(_ context.Context, _, rev string) (string, error) {
			gotRev = rev

			return "c1", testCase.commitErr
		},
		log: slog.New(slog.NewTextHandler(&logged, nil)),
	}

	err := keep.keep(context.Background(), args, testCase.err)
	if (err == nil) != (testCase.err == nil) {
		t.Fatalf("keep() = %v, want error %v", err, testCase.err)
	}

	for _, want := range testCase.wantIs {
		if !errors.Is(err, want) {
			t.Fatalf("keep() = %v, want it to wrap %v", err, want)
		}
	}

	for _, refused := range testCase.wantNot {
		if errors.Is(err, refused) {
			t.Fatalf("keep() = %v, must not wrap %v", err, refused)
		}
	}

	if testCase.err != nil && !strings.Contains(err.Error(), testCase.wantText) {
		t.Fatalf("keep() = %v, want the text %q", err, testCase.wantText)
	}

	state, loadErr := anchorstate.Load(dir)
	if (loadErr == nil) != testCase.wantSaved {
		t.Fatalf("Load() = %v, want saved %v", loadErr, testCase.wantSaved)
	}

	if testCase.wantSaved &&
		(state.Commit != "c1" || state.Args[0] != "emit" || len(state.Conflicts) != 1) {
		t.Fatalf("state = %+v", state)
	}

	checkKeepLog(t, testCase, dir, gotRev, logged.String())
}

func checkKeepLog(t *testing.T, testCase keepCase, dir, gotRev, logged string) {
	t.Helper()

	if !testCase.wantSaved {
		if logged != "" {
			t.Fatalf("a stop that was not kept still logged %q", logged)
		}

		return
	}

	wantRev := cmp.Or(testCase.rev, "HEAD")
	if testCase.isRevEmpty {
		wantRev = ""
	}

	if gotRev != wantRev {
		t.Fatalf("the revision resolved = %q, want %q", gotRev, wantRev)
	}

	for _, want := range []string{
		"anchors: conflict", "rev=" + firstRev, "detail=\"undefined: Z\"", "decls=1",
		"anchors: stopped, state kept", "state_dir=" + dir, "conflicts=1",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log %q lacks %q", logged, want)
		}
	}
}

func TestKeep(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	notKept := "not kept"
	stopped := []error{anchoring.ErrStopped}

	testCases := []keepCase{
		{
			name: "a stop is saved and still returned", err: stopError(),
			wantIs: stopped, wantSaved: true,
		},
		{
			name: "a stop with a very long declaration name is saved", wantSaved: true,
			err: stoppedAt(strings.Repeat("L", 300), firstRev), wantIs: stopped,
		},
		{
			name: "a stop of a named revision resolves that revision", err: stopError(),
			rev: "v1.2", wantIs: stopped, wantSaved: true,
		},
		{
			name: "a stop of an empty revision resolves it as the flag took it", err: stopError(),
			isRevEmpty: true, wantIs: stopped, wantSaved: true,
		},
		{name: "no error is none", err: nil},
		{name: "another error passes through unsaved", err: boom, wantIs: []error{boom}},
		{
			name: "a stop that continue returns is left to continue", verb: Verb, err: stopError(),
			wantIs: stopped,
		},
		{
			name: "a stop with no state dir is a usage error, not a stop", withoutDir: true,
			err: stopError(), wantIs: []error{write.ErrUsage}, wantNot: stopped,
			wantText: notKept,
		},
		{
			name: "a revision that cannot be resolved is a failure, not a stop", err: stopError(),
			commitErr: boom, wantIs: []error{boom}, wantNot: append(stopped, write.ErrUsage),
			wantText: notKept,
		},
		{
			name: "a config that cannot be read is a failure, not a stop", err: stopError(),
			hasNoConfig: true, wantIs: []error{os.ErrNotExist},
			wantNot: append(stopped, write.ErrUsage), wantText: notKept,
		},
		{
			name: "a state dir that is a file is a usage error, not a stop", err: stopError(),
			isDirAFile: true, wantIs: []error{write.ErrUsage, anchorstate.ErrForeign},
			wantNot: stopped, wantText: "--state-dir",
		},
		{
			name: "a state of someone else is a usage error, not a stop", err: stopError(),
			hasForeign: true, wantIs: []error{write.ErrUsage, anchorstate.ErrForeign},
			wantNot: stopped, wantText: "fold or wip",
		},
		{
			name: "a config that is the derived file of the state dir is a usage error",
			err:  stopError(), isSelfFed: true, wantIs: []error{write.ErrUsage}, wantNot: stopped,
			wantText: anchorstate.DerivedFile,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runKeep(t, testCase)
		})
	}
}

func TestKeepPassesWhatIsNotAStopThrough(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	testCases := []struct {
		name string
		args []string
		err  error
	}{
		{name: "no error", args: []string{"emit"}},
		{name: "an error that is not a stop", args: []string{"emit"}, err: boom},
		{name: "a stop with no command line", err: stopError()},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Keep(context.Background(), testCase.args, testCase.err, testFlags)
			if got != testCase.err {
				t.Fatalf("Keep() = %v, want %v untouched", got, testCase.err)
			}
		})
	}
}

func TestKeepRefusesACommandLineTheFlagsDoNotRead(t *testing.T) {
	t.Parallel()

	args := []string{"emit", "--nope", "x"}
	keep := keeper{flags: testFlags, log: slog.New(slog.DiscardHandler)}

	err := keep.keep(context.Background(), args, stopError())
	if err == nil || errors.Is(err, anchoring.ErrStopped) ||
		!strings.Contains(err.Error(), "not kept") {
		t.Fatalf("keep() = %v, want a failure that says the stop was not kept, not a stop", err)
	}
}

func TestKeepAgainWhenTheDirectoryCannotBeWritten(t *testing.T) {
	t.Parallel()

	dir, _ := stoppedState(t)
	locked := func(context.Context, []string, io.Writer, io.Writer) error {
		lock(t, dir)

		return stoppedAt("Run", secondRev)
	}
	run := continuer{
		dispatch: locked,
		flags:    testFlags,
		commit:   func(context.Context, string, string) (string, error) { return "c1", nil },
	}

	err := run.run(context.Background(), []string{"--state", dir, "--fold", "3d2d883"}, io.Discard)
	if errors.Is(err, anchoring.ErrStopped) || err == nil ||
		!strings.Contains(err.Error(), "not kept") {
		t.Fatalf("run() = %v, want a failure that says the stop was not kept, not a stop", err)
	}

	state, loadErr := anchorstate.Load(dir)
	if loadErr != nil || len(state.Conflicts) != 2 {
		t.Fatalf("Load() = %+v, %v, want the earlier state untouched", state, loadErr)
	}
}
