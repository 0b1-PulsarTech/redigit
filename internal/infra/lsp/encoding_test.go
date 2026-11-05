package lsp

import (
	"errors"
	"testing"
)

func TestByteToUTF16(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		line       string
		byteOffset int
		want       int
		wantErr    error
	}{
		{name: "empty line, offset zero", line: "", byteOffset: 0, want: 0},
		{name: "ascii", line: "hello", byteOffset: 5, want: 5},
		{name: "ascii midpoint", line: "hello", byteOffset: 3, want: 3},
		// "café": c(1) a(1) f(1) é(2 bytes, 1 utf-16 unit) -> byte 5 is end of string
		{name: "accented latin, past the accent", line: "café", byteOffset: 5, want: 4},
		{name: "accented latin, before the accent", line: "café", byteOffset: 3, want: 3},
		// CJK: each of "日本語" is a 3-byte UTF-8 rune, 1 UTF-16 unit.
		{
			name:       "cjk, one character in",
			line:       "日本語", //nolint:gosmopolitan // CJK test fixture, not locale-sensitive UI text
			byteOffset: 3,
			want:       1,
		},
		{
			name:       "cjk, all three",
			line:       "日本語", //nolint:gosmopolitan // CJK test fixture, not locale-sensitive UI text
			byteOffset: 9,
			want:       3,
		},
		// An emoji outside the BMP is 4 UTF-8 bytes and a UTF-16 surrogate
		// pair, so it counts as 2 units for 1 rune - the exact hazard this
		// conversion exists to get right.
		{name: "emoji surrogate pair", line: "a\U0001F600b", byteOffset: 1, want: 1},
		{name: "emoji surrogate pair, past it", line: "a\U0001F600b", byteOffset: 5, want: 3},
		{name: "emoji surrogate pair, end", line: "a\U0001F600b", byteOffset: 6, want: 4},
		{name: "negative offset", line: "hello", byteOffset: -1, wantErr: ErrOffsetOutOfRange},
		{name: "offset past end", line: "hello", byteOffset: 6, wantErr: ErrOffsetOutOfRange},
		{name: "offset mid rune", line: "café", byteOffset: 4, wantErr: ErrOffsetOutOfRange},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := ByteToUTF16([]byte(testCase.line), testCase.byteOffset)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("got = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestUTF16ToByte(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		line        string
		utf16Offset int
		want        int
		wantErr     error
	}{
		{name: "empty line, offset zero", line: "", utf16Offset: 0, want: 0},
		{name: "ascii", line: "hello", utf16Offset: 5, want: 5},
		{name: "accented latin, whole string", line: "café", utf16Offset: 4, want: 5},
		{
			name:        "cjk, one character in",
			line:        "日本語", //nolint:gosmopolitan // CJK test fixture, not locale-sensitive UI text
			utf16Offset: 1,
			want:        3,
		},
		{
			name:        "cjk, all three",
			line:        "日本語", //nolint:gosmopolitan // CJK test fixture, not locale-sensitive UI text
			utf16Offset: 3,
			want:        9,
		},
		{name: "emoji surrogate pair, before it", line: "a\U0001F600b", utf16Offset: 1, want: 1},
		{name: "emoji surrogate pair, after it", line: "a\U0001F600b", utf16Offset: 3, want: 5},
		{name: "negative offset", line: "hello", utf16Offset: -1, wantErr: ErrOffsetOutOfRange},
		{name: "offset past end", line: "hello", utf16Offset: 6, wantErr: ErrOffsetOutOfRange},
		{
			name:        "offset lands mid surrogate pair",
			line:        "a\U0001F600b",
			utf16Offset: 2,
			wantErr:     ErrOffsetOutOfRange,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := UTF16ToByte([]byte(testCase.line), testCase.utf16Offset)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("got = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestByteUTF16RoundTrip(t *testing.T) {
	t.Parallel()

	// Mixed-script test fixture (accented latin, CJK, emoji), not locale-sensitive UI text.
	line := []byte("go: café 日本語 \U0001F600 end") //nolint:gosmopolitan
	for byteOffset := 0; byteOffset <= len(line); byteOffset++ {
		units, err := ByteToUTF16(line, byteOffset)
		if err != nil {
			continue // not every byte offset is a rune boundary
		}

		back, err := UTF16ToByte(line, units)
		if err != nil {
			t.Fatalf("UTF16ToByte(%d): %v", units, err)
		}

		if back != byteOffset {
			t.Fatalf(
				"round trip byteOffset=%d -> units=%d -> byteOffset=%d",
				byteOffset,
				units,
				back,
			)
		}
	}
}

func TestPathToURI(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		path    string
		want    string
		wantErr error
	}{
		{
			name: "simple absolute path",
			path: "/home/user/main.go",
			want: "file:///home/user/main.go",
		},
		{
			name: "path with spaces",
			path: "/home/user/my project/main.go",
			want: "file:///home/user/my%20project/main.go",
		},
		{name: "relative path rejected", path: "main.go", wantErr: ErrRelativePath},
		{name: "empty path rejected", path: "", wantErr: ErrRelativePath},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := pathToURI(testCase.path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("got = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestURIToPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		uri     string
		want    string
		wantErr error
	}{
		{name: "simple file uri", uri: "file:///home/user/main.go", want: "/home/user/main.go"},
		{
			name: "escaped space",
			uri:  "file:///home/user/my%20project/main.go",
			want: "/home/user/my project/main.go",
		},
		{
			name:    "non file scheme rejected",
			uri:     "http://example.com/main.go",
			wantErr: ErrUnsupportedScheme,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := uriToPath(testCase.uri)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("got = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestURIRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
	}{
		{name: "ascii", path: "/repo/internal/infra/lsp/session.go"},
		{
			name: "unicode",
			path: "/repo/pkg/日本語/main.go", //nolint:gosmopolitan // unicode fixture, not locale-sensitive UI text
		},
		{name: "spaces", path: "/repo/my module/file.go"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			uri, err := pathToURI(testCase.path)
			if err != nil {
				t.Fatalf("pathToURI: %v", err)
			}

			back, err := uriToPath(uri)
			if err != nil {
				t.Fatalf("uriToPath: %v", err)
			}

			if back != testCase.path {
				t.Fatalf("round trip = %q, want %q", back, testCase.path)
			}
		})
	}
}
