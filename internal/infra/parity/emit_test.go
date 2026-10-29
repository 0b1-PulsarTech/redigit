package parity

import (
	"errors"
	"os"
	"testing"
)

func TestLoadEmit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		path       string
		wantRows   int
		wantErr    error
		wantLine   int
		wantChunks int
	}{
		{name: "plain", path: "testdata/emit_small.jsonl", wantRows: 2, wantChunks: 3},
		{
			name:       "gzip",
			path:       writeGzip(t, "e.jsonl.gz"),
			wantRows:   2,
			wantChunks: 3,
		},
		{
			name:     "truncated row",
			path:     "testdata/emit_truncated.jsonl",
			wantErr:  ErrMalformed,
			wantLine: 1,
		},
		{
			name:     "unknown field",
			path:     "testdata/emit_unknown_field.jsonl",
			wantErr:  ErrMalformed,
			wantLine: 1,
		},
		{
			name:     "second value on a line",
			path:     "testdata/emit_trailing_value.jsonl",
			wantErr:  ErrMalformed,
			wantLine: 1,
		},
		{
			name:     "trailing brace",
			path:     "testdata/emit_trailing_brace.jsonl",
			wantErr:  ErrMalformed,
			wantLine: 1,
		},
		{name: "missing file", path: "testdata/absent.jsonl", wantErr: os.ErrNotExist},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows, err := LoadEmit(testCase.path)
			if testCase.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("err = %v, want %v", err, testCase.wantErr)
				}

				if lineErr, isLine := errors.AsType[*LineError](
					err,
				); isLine &&
					lineErr.Line != testCase.wantLine {
					t.Fatalf("line = %d, want %d", lineErr.Line, testCase.wantLine)
				}

				return
			}

			chunks := 0
			for _, row := range rows {
				chunks += len(row.Chunks)
			}

			if len(rows) != testCase.wantRows || chunks != testCase.wantChunks {
				t.Fatalf("rows = %d chunks = %d", len(rows), chunks)
			}
		})
	}
}

func TestLoadEmitFields(t *testing.T) {
	t.Parallel()

	rows, err := LoadEmit("testdata/emit_small.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	first, second := rows[0], rows[1]
	if first.Feature != "f" || first.Chunks[0].Span != [2]int{0, 10} ||
		first.Blobs[0].Handle != "abc" {
		t.Fatalf("first row = %+v", first)
	}

	if !second.IsWIP || second.Position != 1 || second.Risks[0].BornAt != 3 {
		t.Fatalf("second row = %+v", second)
	}
}

func TestStreamEmitStopsEarly(t *testing.T) {
	t.Parallel()

	seen := 0

	for _, err := range StreamEmit("testdata/emit_small.jsonl") {
		if err != nil {
			t.Fatal(err)
		}

		seen++

		break
	}

	if seen != 1 {
		t.Fatalf("seen = %d", seen)
	}
}

func TestStreamEmitCloseError(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")

	testCases := []struct {
		name      string
		stopAfter int
		wantErr   bool
	}{
		{name: "consumer breaks early", stopAfter: 1},
		{name: "consumer reads to the end", wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var gotErr error

			seen := 0

			for _, err := range streamEmit(closeFailingOpener(closeErr), "testdata/emit_small.jsonl") {
				if err != nil {
					gotErr = err

					continue
				}

				seen++
				if seen == testCase.stopAfter {
					break
				}
			}

			if got := errors.Is(gotErr, closeErr); got != testCase.wantErr {
				t.Fatalf("close error seen = %v, want %v (err %v)", got, testCase.wantErr, gotErr)
			}
		})
	}
}
