package parity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestLoadHardEdges(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		want     []Edge
		wantErr  error
		wantLine int
	}{
		{
			name: "keeps hard drops soft and self edges",
			path: "testdata/edges_small.tsv",
			want: []Edge{
				{"a.go:func:A", "go.mod:other:<file>", entities.ReasonModule},
				{"b.go:func:B", "a.go:func:A", entities.ReasonReference},
			},
		},
		{
			name: "implements is hard only for a non-mock function, statement or method on a method",
			path: "testdata/edges_implements.tsv",
			want: []Edge{
				{"a.go:func:Use", "b.go:method:T.M", entities.ReasonImplements},
				{"a.go:stmt:s", "b.go:method:T.M", entities.ReasonImplements},
				{"a.go:method:V.N", "b.go:method:T.M", entities.ReasonImplements},
			},
		},
		{name: "short line", path: "testdata/edges_short.tsv", wantErr: ErrMalformed, wantLine: 1},
		{
			name:    "bad line after valid ones",
			path:    "testdata/edges_late_bad.tsv",
			wantErr: ErrMalformed, wantLine: 3,
		},
		{name: "missing file", path: "testdata/absent.tsv", wantErr: os.ErrNotExist},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := LoadHardEdges(testCase.path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if lineErr, isLine := errors.AsType[*LineError](
				err,
			); isLine &&
				lineErr.Line != testCase.wantLine {
				t.Fatalf("line = %d, want %d", lineErr.Line, testCase.wantLine)
			}

			if len(got) != len(testCase.want) {
				t.Fatalf("got %v, want %v", got, testCase.want)
			}

			for idx := range got {
				if got[idx] != testCase.want[idx] {
					t.Fatalf("edge %d = %v, want %v", idx, got[idx], testCase.want[idx])
				}
			}
		})
	}
}

func TestStreamEdgesStopsEarly(t *testing.T) {
	t.Parallel()

	seen := 0

	for _, err := range StreamEdges("testdata/edges_small.tsv") {
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

func TestStreamEdgesMidStreamError(t *testing.T) {
	t.Parallel()

	var (
		seen    int
		gotErr  error
		lastErr *LineError
	)

	for _, err := range StreamEdges("testdata/edges_late_bad.tsv") {
		if err != nil {
			gotErr = err

			continue
		}

		seen++
	}

	if seen != 2 || !errors.Is(gotErr, ErrMalformed) {
		t.Fatalf("seen = %d err = %v", seen, gotErr)
	}

	if !errors.As(gotErr, &lastErr) || lastErr.Line != 3 {
		t.Fatalf("err = %v, want line 3", gotErr)
	}
}

func TestStreamEdgesCloseError(t *testing.T) {
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

			for _, err := range streamEdges(closeFailingOpener(closeErr), "testdata/edges_small.tsv") {
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

func TestLoadHardEdgesText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		want    []Edge
	}{
		{
			name:    "crlf parses like lf",
			content: "a.go:func:A\tb.go:func:B\treference\r\nc.go:func:C\td.go:func:D\tmodule\r\n",
			want: []Edge{
				{"a.go:func:A", "b.go:func:B", entities.ReasonReference},
				{"c.go:func:C", "d.go:func:D", entities.ReasonModule},
			},
		},
		{
			name:    "lf baseline",
			content: "a.go:func:A\tb.go:func:B\treference\nc.go:func:C\td.go:func:D\tmodule\n",
			want: []Edge{
				{"a.go:func:A", "b.go:func:B", entities.ReasonReference},
				{"c.go:func:C", "d.go:func:D", entities.ReasonModule},
			},
		},
		{
			name:    "both endpoints empty is a self edge and dropped",
			content: "\t\treference\n",
		},
		{
			name:    "one empty endpoint is kept as an empty string",
			content: "\tb.go:func:B\treference\na.go:func:A\t\tmodule\n",
			want: []Edge{
				{"", "b.go:func:B", entities.ReasonReference},
				{"a.go:func:A", "", entities.ReasonModule},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "edges.tsv")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := LoadHardEdges(path)
			if err != nil {
				t.Fatal(err)
			}

			if len(got) != len(testCase.want) {
				t.Fatalf("got %v, want %v", got, testCase.want)
			}

			for idx := range got {
				if got[idx] != testCase.want[idx] {
					t.Fatalf("edge %d = %q, want %q", idx, got[idx], testCase.want[idx])
				}
			}
		})
	}
}

func TestLoadHardEdgesInternSurvivesBufferReuse(t *testing.T) {
	t.Parallel()

	const lineCount = 100_000

	var content strings.Builder

	for idx := range lineCount {
		fmt.Fprintf(&content, "dep-%d.go:func:F\tshared-provider.go:func:P\treference\n", idx)
	}

	if content.Len() < 2*initialLineBytes {
		t.Fatalf("fixture is %d bytes, too small to reuse the buffer", content.Len())
	}

	path := filepath.Join(t.TempDir(), "edges.tsv")
	if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadHardEdges(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != lineCount {
		t.Fatalf("got %d edges, want %d", len(got), lineCount)
	}

	for idx, edge := range got {
		want := Edge{
			Dependent: fmt.Sprintf("dep-%d.go:func:F", idx),
			Provider:  "shared-provider.go:func:P",
			Reason:    entities.ReasonReference,
		}
		if edge != want {
			t.Fatalf("edge %d = %q, want %q", idx, edge, want)
		}
	}
}
