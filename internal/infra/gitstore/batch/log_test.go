//go:build unix

package batch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestLog(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "1\n2\n3\n4\n5\n6\n7\n8\n"})
	gitIn(t, repo, "mv", "a.go", "b.go")
	gitIn(t, repo, "commit", "-q", "-m", "two")

	grown := "1\n2\n3\n4\n5\n6\n7\n8\n9\n"
	commitFiles(t, repo, "three", map[string]string{"b.go": grown, "sp ace/\u00e9.go": "x\n"})
	gitIn(t, repo, "rm", "-q", "b.go")
	gitIn(t, repo, "commit", "-q", "-m", "four")

	var commits []Commit

	for commit, err := range Log(t.Context(), repo, "main") {
		if err != nil {
			t.Fatalf("Log() err = %v", err)
		}

		commits = append(commits, commit)
	}

	testCases := []struct {
		name       string
		commit     int
		wantStatus byte
		wantPath   string
	}{
		{name: "add", commit: 0, wantStatus: 'A', wantPath: "a.go"},
		{name: "rename", commit: 1, wantStatus: 'R', wantPath: "b.go"},
		{name: "unicode path", commit: 2, wantStatus: 'A', wantPath: "sp ace/\u00e9.go"},
		{name: "delete", commit: 3, wantStatus: 'D', wantPath: "b.go"},
	}

	if len(commits) != 4 {
		t.Fatalf("got %d commits, want 4", len(commits))
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			found := false

			for _, entry := range commits[testCase.commit].Entries {
				if entry.Status == testCase.wantStatus && entry.NewPath == testCase.wantPath {
					found = true
				}
			}

			if !found {
				t.Fatalf("commit %d lacks %c %s: %+v", testCase.commit, testCase.wantStatus,
					testCase.wantPath, commits[testCase.commit].Entries)
			}
		})
	}
}

func TestLogFailures(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "1\n"})

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	testCases := []struct {
		name    string
		ctx     context.Context
		repo    string
		ref     string
		wantErr error
	}{
		{
			name: "unknown ref",
			ctx:  context.Background(), repo: repo, ref: "nope", wantErr: ErrProcess,
		},
		{
			name: "not a repo", ctx: context.Background(), repo: t.TempDir(),
			ref: "main", wantErr: ErrProcess,
		},
		{name: "canceled", ctx: canceled, repo: repo, ref: "main", wantErr: context.Canceled},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var gotErr error

			for _, err := range Log(testCase.ctx, testCase.repo, testCase.ref) {
				if err != nil {
					gotErr = err
				}
			}

			if !errors.Is(gotErr, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", gotErr, testCase.wantErr)
			}
		})
	}
}

func TestLogStopsEarly(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "1\n"})
	commitFiles(t, repo, "two", map[string]string{"a.go": "2\n"})

	seen := 0

	for _, err := range Log(t.Context(), repo, "main") {
		if err != nil {
			t.Fatalf("Log() err = %v", err)
		}

		seen++

		break
	}

	if seen != 1 {
		t.Fatalf("seen = %d, want 1", seen)
	}
}

func TestLogTreatsDashRefAsRevision(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "1\n"})

	planted := filepath.Join(t.TempDir(), "planted")

	var gotErr error

	for _, err := range Log(t.Context(), repo, "--output="+planted) {
		if err != nil {
			gotErr = err
		}
	}

	if !errors.Is(gotErr, ErrProcess) {
		t.Fatalf("err = %v, want ErrProcess", gotErr)
	}

	if _, err := os.Stat(planted); err == nil {
		t.Fatal("a ref starting with a dash was run as an option")
	}
}

var fakeGitDir string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "fakegit")
	if err != nil {
		fmt.Fprintln(os.Stderr, "make fake git dir:", err)

		return 1
	}

	defer func() { _ = os.RemoveAll(dir) }()

	build := exec.CommandContext(context.Background(), "go", "build", "-o",
		filepath.Join(dir, "git"), "./testdata/fakegit")
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build fake git: %v\n%s", buildErr, out)

		return 1
	}

	fakeGitDir = dir

	return m.Run()
}

func useFakeGit(t testing.TB, mode, text string) string {
	t.Helper()

	state := t.TempDir()

	for _, name := range []string{"gate", "ready"} {
		if err := syscall.Mkfifo(filepath.Join(state, name), 0o600); err != nil {
			t.Fatalf("mkfifo %s: %v", name, err)
		}
	}

	t.Setenv("PATH", fakeGitDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKEGIT_DIR", state)
	t.Setenv("FAKEGIT_MODE", mode)
	t.Setenv("FAKEGIT_TEXT", strconv.Quote(text))

	return state
}

func TestLogWaitsForGitToExit(t *testing.T) {
	state := useFakeGit(t, "linger", "")

	var (
		gotErr  error
		commits int
	)

	finished := make(chan struct{})

	go func() {
		defer close(finished)

		for _, err := range Log(t.Context(), t.TempDir(), "main") {
			if err != nil {
				gotErr = err
			}

			commits++
		}
	}()

	// Opening the gate blocks until git has closed stdout and is lingering.
	gate, err := os.OpenFile(filepath.Join(state, "gate"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open gate: %v", err)
	}

	if err = gate.Close(); err != nil {
		t.Fatalf("close gate: %v", err)
	}

	<-finished

	if gotErr != nil || commits != 0 {
		t.Fatalf("Log() = %d commits, err %v; want a clean empty log", commits, gotErr)
	}

	if _, err = os.Stat(filepath.Join(state, "done")); err != nil {
		t.Fatalf("git was not left to finish its exit: %v", err)
	}

	assertReaped(t, state)
}

func TestLogHoldsBackATruncatedLastCommit(t *testing.T) {
	const entry = ":100644 100644 " +
		"1111111111111111111111111111111111111111 " +
		"2222222222222222222222222222222222222222 M\x00"

	useFakeGit(t, "fail", "\x01aaa 1\x00"+entry+"a.go\x00\x01bbb 2\x00"+entry+"b.go\x00")

	var (
		oids   []string
		gotErr error
	)

	for commit, err := range Log(t.Context(), t.TempDir(), "main") {
		if err != nil {
			gotErr = err

			continue
		}

		oids = append(oids, commit.OID)
	}

	if !errors.Is(gotErr, ErrProcess) || !strings.Contains(gotErr.Error(), "fatal: boom") {
		t.Fatalf("err = %v, want ErrProcess carrying stderr", gotErr)
	}

	if !slices.Equal(oids, []string{"aaa"}) {
		t.Fatalf("yielded %v, want only the complete commit aaa", oids)
	}
}

func TestLogIgnoresRenameConfig(t *testing.T) {
	t.Parallel()

	for _, setting := range []string{"false", "true", "copies"} {
		t.Run(setting, func(t *testing.T) {
			t.Parallel()

			repo := newRepo(t)
			gitIn(t, repo, "config", "diff.renames", setting)
			commitFiles(t, repo, "one", map[string]string{"a.go": "1\n2\n3\n4\n5\n6\n7\n8\n"})
			gitIn(t, repo, "mv", "a.go", "b.go")
			gitIn(t, repo, "commit", "-q", "-m", "two")

			var entries []Entry

			for commit, err := range Log(t.Context(), repo, "main") {
				if err != nil {
					t.Fatalf("Log() err = %v", err)
				}

				entries = append(entries, commit.Entries...)
			}

			want := []Entry{
				{Status: 'A', NewPath: "a.go", OldPath: "a.go"},
				{Status: 'R', Similarity: 100, OldPath: "a.go", NewPath: "b.go"},
			}

			if len(entries) != len(want) {
				t.Fatalf("entries = %+v, want %d", entries, len(want))
			}

			for idx, entry := range entries {
				if entry.Status != want[idx].Status || entry.OldPath != want[idx].OldPath ||
					entry.NewPath != want[idx].NewPath || entry.Similarity != want[idx].Similarity {
					t.Fatalf("entry %d = %+v, want %+v", idx, entry, want[idx])
				}
			}
		})
	}
}

func TestLogParseErrorCarriesGitFailure(t *testing.T) {
	useFakeGit(t, "fail", "\x01aaa 1\x00:100644 100644 1111")

	var (
		oids   []string
		gotErr error
	)

	for commit, err := range Log(t.Context(), t.TempDir(), "main") {
		if err != nil {
			gotErr = err

			continue
		}

		oids = append(oids, commit.OID)
	}

	if !errors.Is(gotErr, ErrMalformedLog) || !errors.Is(gotErr, ErrProcess) ||
		!strings.Contains(gotErr.Error(), "boom") {
		t.Fatalf("err = %v, want the parse error joined with git's failure and stderr", gotErr)
	}

	if len(oids) != 0 {
		t.Fatalf("yielded %v, want no truncated commit", oids)
	}
}

func TestLogParseFailureEndings(t *testing.T) {
	const malformed = "\x01aaa 1\x00garbage\x00"

	testCases := []struct {
		name       string
		mode       string
		wantNotErr error
	}{
		{name: "git exits cleanly after the bad record", mode: "exit0", wantNotErr: ErrProcess},
		{name: "git keeps writing past a pipe of output", mode: "flood"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := useFakeGit(t, testCase.mode, malformed)

			var gotErr error

			for _, err := range Log(t.Context(), t.TempDir(), "main") {
				if err != nil {
					gotErr = err
				}
			}

			if !errors.Is(gotErr, ErrMalformedLog) {
				t.Fatalf("err = %v, want ErrMalformedLog", gotErr)
			}

			if testCase.wantNotErr != nil && errors.Is(gotErr, testCase.wantNotErr) {
				t.Fatalf("err = %v, must not be %v", gotErr, testCase.wantNotErr)
			}

			if testCase.mode != "flood" {
				return
			}

			assertReaped(t, state)
		})
	}
}

func TestLogCancelMidRecordIsACancellation(t *testing.T) {
	state := useFakeGit(t, "midrecord", "\x01aaa 1\x00:100644 100644 1111")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		var gotErr error

		for _, err := range Log(ctx, t.TempDir(), "main") {
			if err != nil {
				gotErr = err
			}
		}

		done <- gotErr
	}()

	ready, err := os.OpenFile(filepath.Join(state, "ready"), os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open ready: %v", err)
	}

	t.Cleanup(func() { _ = ready.Close() })

	cancel()

	gotErr := <-done
	if !errors.Is(gotErr, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", gotErr)
	}

	if !errors.Is(gotErr, ErrMalformedLog) {
		t.Fatalf("err = %v, want the parse cause kept", gotErr)
	}
}

func assertReaped(t testing.TB, state string) {
	t.Helper()

	pidText, err := os.ReadFile(filepath.Join(state, "pid"))
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}

	pid, err := strconv.Atoi(string(pidText))
	if err != nil {
		t.Fatalf("parse pid: %v", err)
	}

	if !isReaped(pid) {
		t.Fatal("Log returned without reaping git")
	}
}
