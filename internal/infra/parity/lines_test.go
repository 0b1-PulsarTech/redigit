package parity

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeGzip(t testing.TB, name string) string {
	t.Helper()

	raw, err := os.ReadFile("testdata/emit_small.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var packed bytes.Buffer

	writer := gzip.NewWriter(&packed)
	if _, err = writer.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}

	if err = writer.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	path := filepath.Join(t.TempDir(), name)
	if err = os.WriteFile(path, packed.Bytes(), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

func TestForEachLine(t *testing.T) {
	t.Parallel()

	corrupt := filepath.Join(t.TempDir(), "corrupt.gz")
	if err := os.WriteFile(corrupt, []byte{0x1f, 0x8b, 0x00, 0x00}, 0o600); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name      string
		path      string
		wantLines int
		wantErr   error
		wantLine  int
	}{
		{name: "plain skips blank lines", path: "testdata/emit_small.jsonl", wantLines: 2},
		{
			name:      "gzip is transparent",
			path:      writeGzip(t, "x.jsonl.gz"),
			wantLines: 2,
		},
		{
			name:      "gzip is detected by magic bytes, not suffix",
			path:      writeGzip(t, "x.jsonl"),
			wantLines: 2,
		},
		{
			name:      "a gz suffix on plain text is read as plain text",
			path:      writePlainAs(t, "x.jsonl.gz"),
			wantLines: 2,
		},
		{name: "missing file", path: "testdata/absent.jsonl", wantErr: os.ErrNotExist},
		{name: "corrupt gzip header", path: corrupt, wantErr: ErrMalformed, wantLine: 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			lines := 0
			err := forEachLine(testCase.path, func([]byte) error {
				lines++

				return nil
			})

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if lines != testCase.wantLines {
				t.Fatalf("lines = %d, want %d", lines, testCase.wantLines)
			}

			if lineErr, isLine := errors.AsType[*LineError](
				err,
			); isLine &&
				lineErr.Line != testCase.wantLine {
				t.Fatalf("line = %d, want %d", lineErr.Line, testCase.wantLine)
			}
		})
	}
}

func TestLineErrorNamesFileAndLine(t *testing.T) {
	t.Parallel()

	err := forEachLine("testdata/emit_small.jsonl", func([]byte) error { return ErrMalformed })

	lineErr, isLine := errors.AsType[*LineError](err)
	if !isLine || lineErr.Line != 1 || lineErr.File != "testdata/emit_small.jsonl" {
		t.Fatalf("err = %v, want line error at line 1", err)
	}

	if got := lineErr.Error(); got != "testdata/emit_small.jsonl:1: malformed fixture line" {
		t.Fatalf("message = %q", got)
	}
}

func writePlainAs(t testing.TB, name string) string {
	t.Helper()

	raw, err := os.ReadFile("testdata/emit_small.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	path := filepath.Join(t.TempDir(), name)
	if err = os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

func TestForEachLineScannerErrors(t *testing.T) {
	t.Parallel()

	packed, err := os.ReadFile(writeGzip(t, "full.jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}

	truncated := filepath.Join(t.TempDir(), "truncated.jsonl.gz")
	if err = os.WriteFile(truncated, packed[:len(packed)-12], 0o600); err != nil {
		t.Fatal(err)
	}

	overlong := filepath.Join(t.TempDir(), "overlong.jsonl")
	if err = os.WriteFile(
		overlong, append(bytes.Repeat([]byte{'x'}, maxLineBytes+1), '\n'), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name     string
		path     string
		wantErr  error
		wantLine int
	}{
		{"line over the buffer cap", overlong, bufio.ErrTooLong, 1},
		{"gzip body cut short", truncated, io.ErrUnexpectedEOF, 4},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := forEachLine(testCase.path, func([]byte) error { return nil })
			if !errors.Is(err, ErrMalformed) || !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want ErrMalformed wrapping %v", err, testCase.wantErr)
			}

			lineErr, isLine := errors.AsType[*LineError](err)
			if !isLine || lineErr.Line != testCase.wantLine {
				t.Fatalf("err = %v, want line %d", err, testCase.wantLine)
			}
		})
	}
}

func closeFailingOpener(closeErr error) fixtureOpener {
	return func(path string) (io.Reader, func() error, error) {
		source, closeAll, err := openFixture(path)
		if err != nil {
			return nil, nil, err
		}

		return source, func() error { return errors.Join(closeAll(), closeErr) }, nil
	}
}
