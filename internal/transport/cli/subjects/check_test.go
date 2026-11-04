package subjects

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

func TestCheckCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		subjects  string
		extra     []string
		wantErr   error
		wantLines []string
	}{
		{
			name:     "a clean file passes in silence",
			subjects: `{"0":"` + goodZero + `","1":"` + goodOne + `"}`,
		},
		{
			name:      "an uncovered draft is reported and fails",
			subjects:  `{"0":"` + goodZero + `"}`,
			wantErr:   ErrFindings,
			wantLines: []string{`"position":1,"kind":"uncovered"`},
		},
		{
			name:      "a marker on a plain row fails",
			subjects:  `{"0":":bug: fix(a): [wip] Hand","1":"` + goodOne + `"}`,
			wantErr:   ErrFindings,
			wantLines: []string{`"position":0,"kind":"wip_marker"`},
		},
		{
			name:      "the limit flags apply",
			subjects:  `{"0":"` + goodZero + `","1":"` + goodOne + `"}`,
			extra:     []string{"--max-description", "5"},
			wantErr:   ErrFindings,
			wantLines: []string{`"kind":"description_length"`},
		},
		{
			name:     "a limit of zero turns it off",
			subjects: `{"0":"` + goodZero + `","1":"` + goodOne + `"}`,
			extra:    []string{"--max-description", "0", "--max-line", "0"},
		},
		{
			name:     "subjects that are not an object",
			subjects: `[1]`,
			wantErr:  rewriting.ErrSubjects,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			manifestPath := writeTemp(t, "m.jsonl", rowZero+rowOne)
			subjectsPath := writeTemp(t, "s.json", testCase.subjects)
			args := append(
				[]string{"check", "--manifest", manifestPath, "--subjects", subjectsPath},
				testCase.extra...,
			)

			var stdout bytes.Buffer

			err := Run(context.Background(), args, &stdout)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Run() err = %v, want %v", err, testCase.wantErr)
			}

			for _, want := range testCase.wantLines {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want it to hold %s", stdout.String(), want)
				}
			}

			if len(testCase.wantLines) == 0 && stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want none", stdout.String())
			}
		})
	}
}

func TestCheckCommandMarkerForms(t *testing.T) {
	t.Parallel()

	testCases := []struct{ name, subject string }{
		{name: "closing line one", subject: `:bug: fix(b): [wip]\nHand`},
		{name: "doubled", subject: `:bug: fix(b): [wip] [wip] Two`},
		{name: "ahead of the type", subject: `:bug: [wip] fix(b): Old`},
		{name: "ahead of the emoji", subject: `[wip] :bug: fix(b): Lead`},
		{name: "spaced scope", subject: `:bug: fix(my scope): [wip] Space`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			manifestPath := writeTemp(t, "m.jsonl", rowZero+rowOne)
			clean := writeTemp(t, "c.json", `{"0":"`+testCase.subject+`","1":"`+goodOne+`"}`)
			wip := writeTemp(t, "w.json", `{"0":"`+goodZero+`","1":"`+testCase.subject+`"}`)

			var stdout bytes.Buffer

			args := []string{"check", "--manifest", manifestPath, "--subjects", clean}
			if err := Run(context.Background(), args, &stdout); !errors.Is(err, ErrFindings) ||
				!strings.Contains(stdout.String(), `"position":0,"kind":"wip_marker"`) {
				t.Fatalf("clean row: err = %v, stdout = %q", err, stdout.String())
			}

			stdout.Reset()

			args = []string{"check", "--manifest", manifestPath, "--subjects", wip}
			if err := Run(context.Background(), args, &stdout); err != nil || stdout.Len() != 0 {
				t.Fatalf("wip row: err = %v, stdout = %q, want silence", err, stdout.String())
			}
		})
	}
}

func TestCheckCommandMissingFiles(t *testing.T) {
	t.Parallel()

	present := writeTemp(t, "m.jsonl", rowZero)
	absent := filepath.Join(t.TempDir(), "absent")

	testCases := []struct {
		name string
		args []string
	}{
		{name: "manifest", args: []string{"check", "--manifest", absent, "--subjects", present}},
		{name: "subjects", args: []string{"check", "--manifest", present, "--subjects", absent}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer

			if err := Run(
				context.Background(),
				testCase.args,
				&stdout,
			); !errors.Is(
				err,
				fs.ErrNotExist,
			) {
				t.Fatalf("Run() err = %v, want %v", err, fs.ErrNotExist)
			}
		})
	}
}

func TestCheckParityFixtureIsInformational(t *testing.T) {
	t.Parallel()

	dir := os.Getenv("REDIGIT_PARITY_DIR")
	manifestPath := filepath.Join(dir, "r21k_x.jsonl")
	subjectsPath := filepath.Join(dir, "subjects23-final", "subjects-all.json")

	for _, path := range []string{manifestPath, subjectsPath} {
		if _, err := os.Stat(path); dir == "" || err != nil {
			t.Skipf("parity fixture %s is not available", path)
		}
	}

	var stdout bytes.Buffer

	args := []string{"check", "--manifest", manifestPath, "--subjects", subjectsPath}
	err := Run(context.Background(), args, &stdout)
	t.Logf(
		"check of the parity fixture: err = %v, %d finding lines",
		err,
		strings.Count(stdout.String(), "\n"),
	)
}

func TestCheckCommandWriteFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		subjects string
	}{
		{name: "a finding line cannot be written", subjects: `{"0":"` + goodZero + `"}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			args := []string{
				"check", "--manifest", writeTemp(t, "m.jsonl", rowZero+rowOne),
				"--subjects", writeTemp(t, "s.json", testCase.subjects),
			}

			if err := Run(context.Background(), args, failingWriter{}); !errors.Is(
				err,
				errWriterBroken,
			) {
				t.Fatalf("Run() err = %v, want %v", err, errWriterBroken)
			}
		})
	}
}
