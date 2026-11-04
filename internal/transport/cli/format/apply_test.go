package format

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
)

func encodeShape(t *testing.T, rows []manifest.Row) string {
	t.Helper()

	for index := range rows {
		rows[index].Blobs = nil
	}

	var buffer bytes.Buffer
	if err := manifest.Encode(&buffer, rows); err != nil {
		t.Fatal(err)
	}

	return buffer.String()
}

func decodeFile(t *testing.T, name string) []manifest.Row {
	t.Helper()

	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = file.Close() }()

	rows, err := manifest.Decode(file)
	if err != nil {
		t.Fatal(err)
	}

	return rows
}

type executeCase struct {
	name        string
	rows        []string
	blobs       map[string]string
	config      string
	marker      string
	extra       []string
	cancel      bool
	wantErr     error
	wantSummary string
	wantHandles map[string]string
	wantBlobs   map[string]string
}

func TestExecuteFormatsOnlyIntermediateBlobs(t *testing.T) {
	t.Parallel()

	const (
		rowOne = `{"position":1,"subject":"first","shape":"feat","feature":"f1","reason":"why",` +
			`"is_wip":true,"chunks":[{"key":"k1","span":[1,5],"unit":"fn"}],` +
			`"blobs":[{"path":"a.go","mode":"100644","handle":"h1"},` +
			`{"path":"README.md","mode":"100644","handle":"h3"}],` +
			`"risks":[{"chunk":"k1","target":"t","born_at":2}]}`
		rowTwo = `{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"},` +
			`{"path":"README.md","mode":"100644","handle":"h3"}]}`
		rowThree = `{"position":3,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"},` +
			`{"path":"README.md","mode":"100644","handle":"h4"}]}`
	)

	testCases := []executeCase{
		{
			name:   "fields round-trip and only a non-final handle changes",
			rows:   []string{rowOne, rowTwo, rowThree},
			blobs:  map[string]string{"h1": "lower", "h2": "fin", "h3": "doc", "h4": "doc2"},
			config: fakeConfig,
			wantSummary: `{"blobs":6,"handles":4,"claimed":1,"changed":1,"unchanged":0,` +
				`"failed":0,"passed_through":1,"final_left_alone":2}`,
			wantHandles: map[string]string{
				"1/a.go": sumLower(), "1/README.md": "h3", "2/a.go": sumLower(),
				"2/README.md": "h3", "3/a.go": "h2", "3/README.md": "h4",
			},
			wantBlobs: map[string]string{
				sumLower(): "LOWER", "h2": "fin", "h3": "doc", "h4": "doc2",
			},
		},
		{
			name: "a lone version is the final blob and stays raw",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
			},
			blobs:  map[string]string{"h1": "lower"},
			config: fakeConfig,
			wantSummary: `{"blobs":1,"handles":1,"claimed":0,"changed":0,"unchanged":0,` +
				`"failed":0,"passed_through":0,"final_left_alone":1}`,
			wantHandles: map[string]string{"1/a.go": "h1"},
			wantBlobs:   map[string]string{"h1": "lower"},
		},
		{
			name: "a final blob sharing a handle with an earlier row stays raw",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
			},
			blobs:  map[string]string{"h1": "lower"},
			config: fakeConfig,
			wantSummary: `{"blobs":2,"handles":1,"claimed":1,"changed":1,"unchanged":0,` +
				`"failed":0,"passed_through":0,"final_left_alone":1}`,
			wantHandles: map[string]string{"1/a.go": sumLower(), "2/a.go": "h1"},
			wantBlobs:   map[string]string{"h1": "lower", sumLower(): "LOWER"},
		},
	}

	runExecute(t, testCases)
}

func TestExecuteSettlesEachDistinctBlob(t *testing.T) {
	t.Parallel()

	testCases := []executeCase{
		{
			name: "an already formatted blob keeps its handle and counts as unchanged",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`,
			},
			blobs:  map[string]string{"h1": "UP", "h2": "fin"},
			config: fakeConfig,
			wantSummary: `{"blobs":2,"handles":2,"claimed":1,"changed":0,"unchanged":1,` +
				`"failed":0,"passed_through":0,"final_left_alone":1}`,
			wantHandles: map[string]string{"1/a.go": "h1", "2/a.go": "h2"},
			wantBlobs:   map[string]string{"h1": "UP", "h2": "fin"},
		},
		{
			name: "one handle at two paths is formatted at both",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"},` +
					`{"path":"b.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"},` +
					`{"path":"b.go","mode":"100644","handle":"h3"}]}`,
			},
			blobs:  map[string]string{"h1": "lower", "h2": "fin", "h3": "fin2"},
			config: fakeConfig,
			wantSummary: `{"blobs":4,"handles":3,"claimed":2,"changed":2,"unchanged":0,` +
				`"failed":0,"passed_through":0,"final_left_alone":2}`,
			wantHandles: map[string]string{
				"1/a.go": sumLower(), "1/b.go": sumLower(), "2/a.go": "h2", "2/b.go": "h3",
			},
			wantBlobs: map[string]string{sumLower(): "LOWER", "h2": "fin", "h3": "fin2"},
		},
		{
			name: "a symlink row keeps its handle when a regular row shares handle and path",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"120000","handle":"h1"}]}`,
				`{"position":3,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`,
			},
			blobs:  map[string]string{"h1": "lower", "h2": "fin"},
			config: fakeConfig,
			wantSummary: `{"blobs":3,"handles":2,"claimed":1,"changed":1,"unchanged":0,` +
				`"failed":0,"passed_through":0,"final_left_alone":1}`,
			wantHandles: map[string]string{"1/a.go": sumLower(), "2/a.go": "h1", "3/a.go": "h2"},
			wantBlobs:   map[string]string{"h1": "lower", sumLower(): "LOWER", "h2": "fin"},
		},
	}

	runExecute(t, testCases)
}

func TestExecuteFailures(t *testing.T) {
	t.Parallel()

	testCases := []executeCase{
		{
			name: "a formatter failure leaves the blob raw",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`,
			},
			blobs:  map[string]string{"h1": "BAD one", "h2": "fin"},
			config: fakeConfig,
			wantSummary: `{"blobs":2,"handles":2,"claimed":1,"changed":0,"unchanged":0,` +
				`"failed":1,"passed_through":0,"final_left_alone":1}`,
			wantHandles: map[string]string{"1/a.go": "h1", "2/a.go": "h2"},
			wantBlobs:   map[string]string{"h1": "BAD one", "h2": "fin"},
		},
		{
			name: "a formatter failure is an error under strict",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`,
			},
			blobs:   map[string]string{"h1": "BAD one", "h2": "fin"},
			config:  fakeConfig,
			extra:   []string{"--strict"},
			wantErr: ErrStrict,
		},
		{
			name: "a failure in the second round of a path leaves only that blob raw",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h3"}]}`,
				`{"position":3,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`,
			},
			blobs:  map[string]string{"h1": "lower", "h3": "BAD two", "h2": "fin"},
			config: fakeConfig,
			wantSummary: `{"blobs":3,"handles":3,"claimed":2,"changed":1,"unchanged":0,` +
				`"failed":1,"passed_through":0,"final_left_alone":1}`,
			wantHandles: map[string]string{"1/a.go": sumLower(), "2/a.go": "h3", "3/a.go": "h2"},
			wantBlobs:   map[string]string{sumLower(): "LOWER", "h3": "BAD two", "h2": "fin"},
		},
		{
			name: "a cancelled run is an error and publishes nothing",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`,
			},
			blobs:   map[string]string{"h1": "lower", "h2": "fin"},
			config:  fakeConfig,
			cancel:  true,
			wantErr: context.Canceled,
		},
	}

	runExecute(t, testCases)
}

func TestExecuteConfigRootFileSteersTheFormatter(t *testing.T) {
	t.Parallel()

	testCases := []executeCase{
		{
			name:   "the root config file sets the tool's mode and an unclaimed copy passes through",
			marker: "LOWER",
			rows: []string{
				`{"position":1,"blobs":[{"path":"fake.cfg","mode":"100644","handle":"h1"},` +
					`{"path":"a.go","mode":"100644","handle":"h3"}]}`,
				`{"position":2,"blobs":[{"path":"fake.cfg","mode":"100644","handle":"h2"},` +
					`{"path":"a.go","mode":"100644","handle":"h4"}]}`,
			},
			blobs: map[string]string{"h1": "k", "h2": "k2", "h3": "ABC", "h4": "fin"},
			config: `{"format":{"config_root":"{root}","copy_files":["fake.cfg"],` +
				`"languages":[{"name":"go","extensions":[".go"],"steps":[` +
				`{"cmd":["fakefmt","-w"],"mode":"files","timeout":"30s"}]}]}}`,
			wantSummary: `{"blobs":4,"handles":4,"claimed":1,"changed":1,"unchanged":0,` +
				`"failed":0,"passed_through":1,"final_left_alone":2}`,
			wantHandles: map[string]string{
				"1/fake.cfg": "h1", "1/a.go": sumOf("abc"), "2/fake.cfg": "h2", "2/a.go": "h4",
			},
			wantBlobs: map[string]string{"h1": "k", sumOf("abc"): "abc"},
		},
		{
			name: "a claimed config file is formatted, and read as its own config",
			rows: []string{
				`{"position":1,"blobs":[{"path":"fake.cfg","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"fake.cfg","mode":"100644","handle":"h2"}]}`,
			},
			blobs: map[string]string{"h1": "ABC LOWER", "h2": "fin"},
			config: `{"format":{"config_root":"{root}","copy_files":["fake.cfg"],` +
				`"languages":[{"name":"go","extensions":[".go",".cfg"],"steps":[` +
				`{"cmd":["fakefmt","-w"],"mode":"files","timeout":"30s"}]}]}}`,
			wantSummary: `{"blobs":2,"handles":2,"claimed":1,"changed":1,"unchanged":0,` +
				`"failed":0,"passed_through":0,"final_left_alone":1}`,
			wantHandles: map[string]string{"1/fake.cfg": sumOf("abc lower"), "2/fake.cfg": "h2"},
			wantBlobs:   map[string]string{sumOf("abc lower"): "abc lower", "h2": "fin"},
		},
	}

	runExecute(t, testCases)
}

func runExecute(t *testing.T, testCases []executeCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			verifyExecute(t, testCase)
		})
	}
}

func verifyExecute(t *testing.T, testCase executeCase) {
	t.Helper()

	fix := stage(t, testCase.rows, testCase.blobs, testCase.config)
	if testCase.marker != "" {
		marked := filepath.Join(filepath.Dir(fix.manifest), "root", "fake.cfg")
		mustWrite(t, marked, testCase.marker)
	}

	before, _ := os.ReadFile(fix.manifest)
	inputRows := decodeFile(t, fix.manifest)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if testCase.cancel {
		cancel()
	}

	var stdout bytes.Buffer

	err := Run(ctx, fix.args(testCase.extra...), &stdout)
	if !errors.Is(err, testCase.wantErr) {
		t.Fatalf("Run() err = %v, want %v", err, testCase.wantErr)
	}

	if testCase.wantErr != nil {
		for _, name := range []string{fix.out, fix.outBlobs} {
			if _, statErr := os.Stat(name); statErr == nil {
				t.Fatalf("a failed run created %s", name)
			}
		}

		return
	}

	if got := strings.TrimSpace(stdout.String()); got != testCase.wantSummary {
		t.Fatalf("summary = %s, want %s", got, testCase.wantSummary)
	}

	after, _ := os.ReadFile(fix.manifest)
	if !bytes.Equal(before, after) {
		t.Fatal("the input manifest changed")
	}

	checkOutputs(t, fix, inputRows, testCase.wantHandles, testCase.wantBlobs)

	names, _ := filepath.Glob(filepath.Join(fix.blobs, "*"))
	if len(names) != len(testCase.blobs)+1 {
		t.Fatalf("the input blob dir holds %v", names)
	}
}

func checkOutputs(
	t *testing.T,
	fix fixture,
	inputRows []manifest.Row,
	wantHandles, wantBlobs map[string]string,
) {
	t.Helper()

	outRows := decodeFile(t, fix.out)
	gotHandles := map[string]string{}

	for _, row := range outRows {
		for _, blob := range row.Blobs {
			gotHandles[fmt.Sprintf("%d/%s", row.Position, blob.Path)] = blob.Handle
		}
	}

	if fmt.Sprint(gotHandles) != fmt.Sprint(wantHandles) {
		t.Fatalf("handles = %v, want %v", gotHandles, wantHandles)
	}

	if encodeShape(t, outRows) != encodeShape(t, inputRows) {
		t.Fatal("a field other than blobs[].handle changed")
	}

	for handle, content := range wantBlobs {
		got, readErr := os.ReadFile(filepath.Join(fix.outBlobs, handle))
		if readErr != nil || string(got) != content {
			t.Fatalf("out blob %s = %q (%v), want %q", handle, got, readErr, content)
		}
	}
}

func TestExecuteCallsTheFormatterOncePerDistinctBlob(t *testing.T) {
	testCases := []struct {
		name      string
		rows      []string
		blobs     map[string]string
		wantCalls int
	}{
		{
			name: "one handle at one path in three rows",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":3,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":4,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`,
			},
			blobs:     map[string]string{"h1": "one", "h2": "fin"},
			wantCalls: 1,
		},
		{
			name: "two handles at one path are two calls",
			rows: []string{
				`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
				`{"position":2,"blobs":[{"path":"a.go","mode":"100644","handle":"h3"}]}`,
				`{"position":3,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`,
			},
			blobs:     map[string]string{"h1": "one", "h2": "fin", "h3": "two"},
			wantCalls: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logFile := filepath.Join(t.TempDir(), "calls.log")
			t.Setenv("FAKEFMT_LOG", logFile)

			fix := stage(t, testCase.rows, testCase.blobs, fakeConfig)

			var stdout bytes.Buffer
			if err := Run(context.Background(), fix.args(), &stdout); err != nil {
				t.Fatalf("Run() err = %v", err)
			}

			logged, _ := os.ReadFile(logFile)
			if got := len(slices.DeleteFunc(
				strings.Split(string(logged), "\n"), func(line string) bool { return line == "" },
			)); got != testCase.wantCalls {
				t.Fatalf("formatter calls = %d, want %d", got, testCase.wantCalls)
			}
		})
	}
}

type failingWriter struct{ err error }

func (sink failingWriter) Write([]byte) (int, error) { return 0, sink.err }

func TestExecuteReportsAFailingSummaryWriter(t *testing.T) {
	t.Parallel()

	fix := stage(t, []string{
		`{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`,
	}, map[string]string{"h1": "x"}, fakeConfig)
	sinkErr := errors.New("sink closed")

	err := Run(context.Background(), fix.args(), failingWriter{err: sinkErr})
	if !errors.Is(err, sinkErr) || !strings.Contains(err.Error(), "write summary") {
		t.Fatalf("Run() err = %v, want write summary wrapping %v", err, sinkErr)
	}
}
