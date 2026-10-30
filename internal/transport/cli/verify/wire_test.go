package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/slots"
)

var (
	errAssemble = errors.New("assemble")
	errWriter   = errors.New("writer")

	_ verifying.Commits = fakeCommits{}
	_ verifying.Modules = fakeModules{}
	_ verifying.Trees   = fakeTrees{}
	_ verifying.Builder = fakeBuilder{}
	_ verifying.Cache   = fakeCache{}
)

type (
	fakeCommits struct{ subjects []string }

	fakeModules struct{}

	fakeTrees struct{ err error }

	fakeBuilder struct {
		failing     map[string]string
		identityErr error
	}

	fakeCache struct{}

	brokenWriter struct{}
)

func (fake fakeCommits) List(context.Context, string) ([]verifying.Commit, error) {
	commits := make([]verifying.Commit, 0, len(fake.subjects))

	for idx, subject := range fake.subjects {
		commits = append(commits, verifying.Commit{
			Oid: "c" + string(rune('1'+idx)), Tree: "t", Subject: subject, Position: idx + 1,
		})
	}

	return commits, nil
}

func (fakeModules) List(_ context.Context, commit verifying.Commit) (verifying.Listing, error) {
	return verifying.Listing{Modules: []verifying.Module{
		{Dir: "mod", Path: "m", Language: "go", Subtree: commit.Oid},
	}}, nil
}

func (fake fakeTrees) Materialise(context.Context, int, verifying.Commit) (string, error) {
	return "slot", fake.err
}

func (fakeTrees) Close(context.Context) error { return nil }

func (fake fakeBuilder) Identity(context.Context, verifying.Unit) (string, error) {
	return "id", fake.identityErr
}

func (fake fakeBuilder) Check(
	_ context.Context,
	_ string,
	unit verifying.Unit,
) (verifying.Outcome, error) {
	if tail, isFailing := fake.failing[unit.Subtree]; isFailing {
		return verifying.Outcome{IsFailed: true, Step: "go build", Tail: tail}, nil
	}

	return verifying.Outcome{Step: "go vet"}, nil
}

func (fakeCache) Get(context.Context, string) (verifying.Outcome, bool, error) {
	return verifying.Outcome{}, false, nil
}

func (fakeCache) Put(context.Context, string, verifying.Outcome) error { return nil }

func (brokenWriter) Write([]byte) (int, error) { return 0, errWriter }

func assembling(commits fakeCommits, builder fakeBuilder, failure error) assembler {
	return func(context.Context, plan) (verifying.Sweep, error) {
		if failure != nil {
			return verifying.Sweep{}, failure
		}

		return verifying.Sweep{
			Commits: commits, Trees: fakeTrees{}, Modules: fakeModules{},
			Builder: builder, Cache: fakeCache{},
		}, nil
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	plain := fakeCommits{subjects: []string{"a", "b", "c"}}
	wip := fakeCommits{subjects: []string{"a", "[wip] b", "c"}}

	testCases := []struct {
		name     string
		args     []string
		assemble assembler
		wantErr  error
		wantRows []string
	}{
		{
			"all green", nil, assembling(plain, fakeBuilder{}, nil),
			nil,
			[]string{"pass", "pass", "pass"},
		},
		{
			"a failing commit", nil,
			assembling(plain, fakeBuilder{failing: map[string]string{"c2": "boom"}}, nil),
			ErrFailed,
			[]string{"pass", "fail", "pass"},
		},
		{
			"a failing wip commit is tolerated", nil,
			assembling(wip, fakeBuilder{failing: map[string]string{"c2": "boom"}}, nil),
			nil,
			[]string{"pass", "fail wip", "pass"},
		},
		{
			"a failing wip commit written by redigit write exits clean", nil,
			assembling(
				fakeCommits{subjects: []string{"a", "feat(x): [wip] b", "c"}},
				fakeBuilder{failing: map[string]string{"c2": "boom"}}, nil,
			),
			nil,
			[]string{"pass", "fail wip", "pass"},
		},
		{
			"builder error", nil,
			assembling(plain, fakeBuilder{identityErr: errWriter}, nil),
			verifying.ErrBuilder, nil,
		},
		{"assemble error", nil, assembling(plain, fakeBuilder{}, errAssemble), errAssemble, nil},
		{"usage error", []string{"--nope"}, assembling(plain, fakeBuilder{}, nil), ErrUsage, nil},
		{
			"from beyond the commit count is a usage error",
			[]string{"--from", "9"},
			assembling(plain, fakeBuilder{}, nil), ErrUsage, nil,
		},
		{
			"a run that checked nothing is not ok",
			[]string{"--exclude", "mod"},
			assembling(plain, fakeBuilder{}, nil),
			ErrFailed,
			[]string{"empty", "empty", "empty"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer

			err := run(context.Background(), testCase.args, &out, testCase.assemble)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("run() err = %v, want %v", err, testCase.wantErr)
			}

			summaryLine := assertRows(t, out.String(), testCase.wantRows)
			if summaryLine != "" {
				assertSummary(t, summaryLine, len(testCase.wantRows), err)
			}
		})
	}
}

func assertRows(t *testing.T, stdout string, wantRows []string) string {
	t.Helper()

	if len(wantRows) == 0 {
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}

		return ""
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != len(wantRows)+1 {
		t.Fatalf("got %d lines, want %d rows and a summary: %q", len(lines), len(wantRows), lines)
	}

	rowKeys := []string{
		"position", "commit", "subject", "module", "language", "status", "step", "wip",
		"cached", "millis",
	}

	for idx, want := range wantRows {
		status, isWip := strings.CutSuffix(want, " wip")
		row := decodeLine(t, lines[idx], rowKeys)

		_, hasTail := row["tail"]
		if row["status"] != status || row["wip"] != isWip || hasTail != (status == "fail") {
			t.Fatalf("row %d = %v, want %s", idx, row, want)
		}
	}

	return lines[len(wantRows)]
}

func assertSummary(t *testing.T, line string, rowCount int, runErr error) {
	t.Helper()

	summary := decodeLine(t, line, []string{
		"summary", "commits", "checked", "passed", "failed", "wip_failed", "empty", "cached",
		"overhead_micros", "elapsed_ms", "ok",
	})

	if wantOK := runErr == nil; summary["ok"] != wantOK || summary["summary"] != true {
		t.Fatalf("summary = %v, want ok %v", summary, wantOK)
	}

	if want := float64(rowCount); summary["commits"] != want {
		t.Fatalf("commits = %v, want %v", summary["commits"], want)
	}
}

func decodeLine(t *testing.T, line string, keys []string) map[string]any {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("line %q: %v", line, err)
	}

	for _, key := range keys {
		if _, found := decoded[key]; !found {
			t.Fatalf("line lacks %q: %v", key, decoded)
		}
	}

	return decoded
}

func TestRunFailureCountsRows(t *testing.T) {
	t.Parallel()

	assemble := assembling(
		fakeCommits{subjects: []string{"a", "b", "c"}},
		fakeBuilder{failing: map[string]string{"c1": "x", "c3": "y"}}, nil,
	)

	var out bytes.Buffer

	err := run(context.Background(), nil, &out, assemble)
	if !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), "2 failing module rows") {
		t.Fatalf("run() err = %v, want ErrFailed counting 2 failing module rows", err)
	}
}

func TestRunNamesAPlatformWithoutSlots(t *testing.T) {
	t.Parallel()

	assemble := func(context.Context, plan) (verifying.Sweep, error) {
		return verifying.Sweep{
			Commits: fakeCommits{subjects: []string{"a"}},
			Trees:   fakeTrees{err: slots.ErrSlotsUnsupported},
			Modules: fakeModules{}, Builder: fakeBuilder{}, Cache: fakeCache{},
		}, nil
	}

	err := run(context.Background(), nil, &bytes.Buffer{}, assemble)
	if !errors.Is(err, slots.ErrSlotsUnsupported) || errors.Is(err, ErrUsage) ||
		!strings.Contains(err.Error(), "needs unix file locks") {
		t.Fatalf("run() err = %v, want a clear refusal that is not a usage error", err)
	}
}

func TestRunWriterFailure(t *testing.T) {
	t.Parallel()

	assemble := assembling(fakeCommits{subjects: []string{"a"}}, fakeBuilder{}, nil)

	err := run(context.Background(), nil, brokenWriter{}, assemble)
	if !errors.Is(err, errWriter) {
		t.Fatalf("run() err = %v, want the writer failure", err)
	}
}

var errPrune = errors.New("prune")

type pruningBuilder struct {
	fakeBuilder

	calls *int
	err   error
}

func (pruning pruningBuilder) Prune(time.Time) error {
	*pruning.calls++

	return pruning.err
}

func TestRunPrunesTheBuildCachesAfterTheSweep(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		failing   map[string]string
		pruneErr  error
		wantErr   error
		wantCalls int
	}{
		{"a green sweep", nil, nil, nil, 1},
		{"a red sweep", map[string]string{"c1": "boom"}, nil, ErrFailed, 1},
		{"a failing prune never fails the run", nil, errPrune, nil, 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var calls int

			builder := pruningBuilder{
				fakeBuilder: fakeBuilder{failing: testCase.failing},
				calls:       &calls, err: testCase.pruneErr,
			}
			assemble := func(context.Context, plan) (verifying.Sweep, error) {
				return verifying.Sweep{
					Commits: fakeCommits{subjects: []string{"a"}}, Trees: fakeTrees{},
					Modules: fakeModules{}, Builder: builder, Cache: fakeCache{},
				}, nil
			}

			err := run(t.Context(), nil, &bytes.Buffer{}, assemble)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("run() err = %v, want %v", err, testCase.wantErr)
			}

			if calls != testCase.wantCalls {
				t.Fatalf("Prune called %d times, want %d", calls, testCase.wantCalls)
			}
		})
	}
}
