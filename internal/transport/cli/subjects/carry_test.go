package subjects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
)

func digestOf(t *testing.T, manifest string) string {
	t.Helper()

	var stdout bytes.Buffer

	args := []string{"digest", "--manifest", writeTemp(t, "m.jsonl", manifest)}
	if err := Run(context.Background(), args, &stdout); err != nil {
		t.Fatalf("digest: %v", err)
	}

	return writeTemp(t, "digest.jsonl", stdout.String())
}

func TestCarryCommand(t *testing.T) {
	t.Parallel()

	similar := `{"position":4,"subject":"DRAFT: feat(a): x","shape":"feature","is_wip":false,` +
		`"chunks":[{"key":"k1","span":[0,9],"unit":"u"},{"key":"k2","span":[9,19],"unit":"u"},` +
		`{"key":"k4","span":[0,3],"unit":"u"}],"blobs":[],"risks":[]}` + "\n"

	testCases := []struct {
		name       string
		next       string
		wantIdent  int
		wantReview int
		wantLost   int
	}{
		{name: "same plan carries everything", next: rowOne + rowZero, wantIdent: 2},
		{
			name:      "a commit below the floor is lost",
			next:      similar + rowOne,
			wantIdent: 1,
			wantLost:  1,
		},
		{
			name:     "an unrelated plan loses all",
			next:     `{"position":0,"chunks":[{"key":"z"}]}` + "\n",
			wantLost: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			oldDigest := digestOf(t, rowZero+rowOne)
			handPath := writeTemp(t, "s.json", `{"0":"`+goodZero+`","1":"`+goodOne+`"}`)
			newPath := writeTemp(t, "n.jsonl", testCase.next)
			outPath := filepath.Join(t.TempDir(), "out.json")
			reviewPath := filepath.Join(t.TempDir(), "review.json")

			var stdout bytes.Buffer

			args := []string{
				"carry", "--from", oldDigest, "--subjects", handPath, "--manifest", newPath,
				"--out", outPath, "--review", reviewPath,
			}
			if err := Run(context.Background(), args, &stdout); err != nil {
				t.Fatalf("Run() err = %v", err)
			}

			var got report
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("decode report %q: %v", stdout.String(), err)
			}

			if got.Sources != 2 || got.Identical != testCase.wantIdent ||
				got.Review != testCase.wantReview || got.Lost != testCase.wantLost {
				t.Fatalf(
					"report = %+v, want %d/%d/%d",
					got,
					testCase.wantIdent,
					testCase.wantReview,
					testCase.wantLost,
				)
			}

			written := readMap(t, outPath)
			if len(written) != testCase.wantIdent {
				t.Fatalf("out = %v, want %d entries", written, testCase.wantIdent)
			}

			if testCase.wantIdent == 2 &&
				written[subjecting.IDPrefix+subjecting.ID([]string{"k3"}, []string{"b.go"})] != goodOne {
				t.Fatalf("out = %v, want id keyed hand subjects", written)
			}

			if len(readMap(t, reviewPath)) != testCase.wantReview {
				t.Fatalf("review file differs from the report %+v", got)
			}
		})
	}
}

func readMap(t *testing.T, path string) map[string]string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var loaded map[string]string
	if err = json.Unmarshal(content, &loaded); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}

	return loaded
}

func TestCarryCommandFailures(t *testing.T) {
	t.Parallel()

	man := writeTemp(t, "m.jsonl", rowZero)
	dig := digestOf(t, rowZero)
	sub := writeTemp(t, "s.json", `{}`)
	gone := filepath.Join(t.TempDir(), "absent")

	testCases := []struct {
		name    string
		from    string
		hand    string
		next    string
		out     string
		wantErr error
	}{
		{
			name:    "digest missing",
			from:    gone,
			hand:    sub,
			next:    man,
			out:     "o.json",
			wantErr: fs.ErrNotExist,
		},
		{
			name:    "digest garbage",
			from:    man,
			hand:    sub,
			next:    man,
			out:     "o.json",
			wantErr: rewriting.ErrManifest,
		},
		{
			name:    "subjects missing",
			from:    dig,
			hand:    gone,
			next:    man,
			out:     "o.json",
			wantErr: fs.ErrNotExist,
		},
		{
			name:    "manifest missing",
			from:    dig,
			hand:    sub,
			next:    gone,
			out:     "o.json",
			wantErr: fs.ErrNotExist,
		},
		{
			name:    "out unwritable",
			from:    dig,
			hand:    sub,
			next:    man,
			out:     gone + "/dir/o.json",
			wantErr: fs.ErrNotExist,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out := testCase.out
			if out == "o.json" {
				out = filepath.Join(t.TempDir(), out)
			}

			var stdout bytes.Buffer

			args := []string{
				"carry", "--from", testCase.from, "--subjects", testCase.hand,
				"--manifest", testCase.next, "--out", out,
			}

			err := Run(context.Background(), args, &stdout)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Run() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestPercent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		part, whole int
		want        float64
	}{
		{name: "no sources", part: 0, whole: 0, want: 0},
		{name: "half", part: 1, whole: 2, want: 50},
		{name: "one decimal", part: 1, whole: 3, want: 33.3},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := percent(testCase.part, testCase.whole); got != testCase.want {
				t.Fatalf("percent() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestCarryCommandReviewTierStaysOutOfOut(t *testing.T) {
	t.Parallel()

	chunks := func(keys ...string) string {
		line := `{"position":0,"subject":"DRAFT: feat(a): x","chunks":[`

		for idx, key := range keys {
			if idx > 0 {
				line += ","
			}

			line += `{"key":"` + key + `","span":[0,1],"unit":"u"}`
		}

		return line + `],"blobs":[]}` + "\n"
	}

	oldDigest := digestOf(t, chunks("a", "b", "c", "d", "e"))
	handPath := writeTemp(t, "s.json", `{"0":"`+goodZero+`"}`)
	newPath := writeTemp(t, "n.jsonl", chunks("a", "b", "c", "d", "e", "f"))
	outPath := filepath.Join(t.TempDir(), "out.json")
	reviewPath := filepath.Join(t.TempDir(), "review.json")

	var stdout bytes.Buffer

	args := []string{
		"carry", "--from", oldDigest, "--subjects", handPath, "--manifest", newPath,
		"--out", outPath, "--review", reviewPath,
	}
	if err := Run(context.Background(), args, &stdout); err != nil {
		t.Fatalf("Run() err = %v", err)
	}

	var got report
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	if got.Identical != 0 || got.Review != 1 || got.ReviewPC != 100 || len(got.ReviewRows) != 1 {
		t.Fatalf("report = %+v, want one review row", got)
	}

	if len(readMap(t, outPath)) != 0 || len(readMap(t, reviewPath)) != 1 {
		t.Fatal("the review tier must reach --review and never --out")
	}
}

func TestCarryCommandRefusesProblems(t *testing.T) {
	t.Parallel()

	zeroID := subjecting.ID([]string{"k1", "k2"}, []string{"a.go"})

	testCases := []struct {
		name      string
		from      string
		subjects  string
		wantNames []string
	}{
		{
			name:      "a stale position and an unknown id",
			from:      rowZero,
			subjects:  `{"0":"A","99":"B","id:beef":"C"}`,
			wantNames: []string{`"Key":"99"`, `"Key":"id:beef"`},
		},
		{
			name:      "an empty digest while the subjects file has keys",
			from:      "",
			subjects:  `{"0":"A","1":"B","id:beef":"C"}`,
			wantNames: []string{`"Key":"0"`, `"Key":"1"`, `"Key":"id:beef"`},
		},
		{
			name:      "a signed position",
			from:      rowZero,
			subjects:  `{"-0":"A"}`,
			wantNames: []string{`"Key":"-0"`},
		},
		{
			name:      "a position and an id reaching one row",
			from:      rowZero,
			subjects:  `{"0":"A","id:` + zeroID + `":"B"}`,
			wantNames: []string{`"Kind":"key_conflict"`},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			oldDigest := digestOf(t, testCase.from)
			handPath := writeTemp(t, "s.json", testCase.subjects)
			newPath := writeTemp(t, "n.jsonl", rowZero)
			outPath := filepath.Join(t.TempDir(), "out.json")
			reviewPath := filepath.Join(t.TempDir(), "review.json")

			var stdout bytes.Buffer

			args := []string{
				"carry", "--from", oldDigest, "--subjects", handPath, "--manifest", newPath,
				"--out", outPath, "--review", reviewPath,
			}

			if err := Run(context.Background(), args, &stdout); !errors.Is(err, ErrFindings) {
				t.Fatalf("Run() err = %v, want %v", err, ErrFindings)
			}

			for _, name := range testCase.wantNames {
				if !strings.Contains(stdout.String(), name) {
					t.Fatalf("stdout = %q, want it to name %s", stdout.String(), name)
				}
			}

			for _, path := range []string{outPath, reviewPath} {
				if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("%s must not be written, stat err = %v", path, err)
				}
			}
		})
	}
}

func TestCarryCommandStripsTheMarker(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		subjects string
		want     string
	}{
		{
			name:     "a marker in place is not carried",
			subjects: `{"1":":gem: feat(b): [wip] Other hand"}`,
			want:     goodOne,
		},
		{
			name:     "a clean subject is carried as it is",
			subjects: `{"1":"` + goodOne + `"}`,
			want:     goodOne,
		},
		{
			name:     "a doubled marker is not carried",
			subjects: `{"1":":gem: feat(b): [wip] [wip] Other hand"}`,
			want:     goodOne,
		},
		{
			name:     "a marker ahead of the emoji is not carried",
			subjects: `{"1":"[wip] :gem: feat(b): Other hand"}`,
			want:     goodOne,
		},
		{
			name:     "a marker closing line one is not carried",
			subjects: `{"1":":gem: feat(b): [wip]\nOther hand"}`,
			want:     goodOne,
		},
		{
			name:     "words are kept",
			subjects: `{"1":":gem: feat(b): Keep the [wip] marker"}`,
			want:     ":gem: feat(b): Keep the [wip] marker",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			oldDigest := digestOf(t, rowZero+rowOne)
			handPath := writeTemp(t, "s.json", testCase.subjects)
			newPath := writeTemp(t, "n.jsonl", rowZero+rowOne)
			outPath := filepath.Join(t.TempDir(), "out.json")

			var stdout bytes.Buffer

			args := []string{
				"carry", "--from", oldDigest, "--subjects", handPath, "--manifest", newPath,
				"--out", outPath,
			}
			if err := Run(context.Background(), args, &stdout); err != nil {
				t.Fatalf("Run() err = %v", err)
			}

			written := readMap(t, outPath)
			key := subjecting.IDPrefix + subjecting.ID([]string{"k3"}, []string{"b.go"})

			if written[key] != testCase.want {
				t.Fatalf("out = %v, want %q under %s", written, testCase.want, key)
			}
		})
	}
}

func TestCarryCommandWriteFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		stdout       io.Writer
		reviewInGone bool
		wantErr      error
	}{
		{name: "the report write fails", stdout: failingWriter{}, wantErr: errWriterBroken},
		{
			name:         "the review file write fails",
			stdout:       &bytes.Buffer{},
			reviewInGone: true,
			wantErr:      fs.ErrNotExist,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			oldDigest := digestOf(t, rowZero)
			handPath := writeTemp(t, "s.json", `{"0":"`+goodZero+`"}`)
			newPath := writeTemp(t, "n.jsonl", rowZero)
			reviewPath := filepath.Join(t.TempDir(), "review.json")

			if testCase.reviewInGone {
				reviewPath = filepath.Join(t.TempDir(), "absent", "review.json")
			}

			args := []string{
				"carry", "--from", oldDigest, "--subjects", handPath, "--manifest", newPath,
				"--out", filepath.Join(t.TempDir(), "out.json"), "--review", reviewPath,
			}

			err := Run(context.Background(), args, testCase.stdout)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Run() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
