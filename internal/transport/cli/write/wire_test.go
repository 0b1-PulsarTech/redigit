package write

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

func TestReportWarnsWhenTheResultLineCannotBeWritten(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		wantParts []string
	}{
		{
			"closed stdout after the move",
			[]string{"level=WARN", "ref=refs/heads/out", "commit=abc", "pipe closed"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			logger := slog.New(slog.NewTextHandler(&logs, nil))

			report(failingWriter{}, logger, "refs/heads/out", rewriting.ReplayResult{Commit: "abc"})

			for _, part := range testCase.wantParts {
				if !strings.Contains(logs.String(), part) {
					t.Fatalf("report() logged %q, want it to hold %q", logs.String(), part)
				}
			}
		})
	}
}

func TestReportPrintsTheResultAndWarnsOnLeftovers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		replayed rewriting.ReplayResult
		wantLog  string
	}{
		{
			"clean run prints the line and stays quiet",
			rewriting.ReplayResult{Commit: "abc", Commits: 2},
			"",
		},
		{
			"leftover scratch ref still prints the line and warns",
			rewriting.ReplayResult{Commit: "abc", Commits: 2, CleanupErr: errors.New("stuck")},
			"stuck",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, logs bytes.Buffer

			logger := slog.New(slog.NewTextHandler(&logs, nil))

			report(&stdout, logger, "refs/heads/out", testCase.replayed)

			const wantLine = `{"ref":"refs/heads/out","commit":"abc","commits":2}` + "\n"
			if stdout.String() != wantLine {
				t.Fatalf("report() printed %q, want %q", stdout.String(), wantLine)
			}

			isWarned := strings.Contains(logs.String(), "level=WARN")
			if isWarned != (testCase.wantLog != "") ||
				!strings.Contains(logs.String(), testCase.wantLog) {
				t.Fatalf("report() logged %q, want it to hold %q", logs.String(), testCase.wantLog)
			}
		})
	}
}
