package indexing

import "testing"

func TestLineIndexLineOf(t *testing.T) {
	t.Parallel()

	index := NewLineIndex([]byte("package a\n\nfunc B() {}\n"))

	testCases := []struct {
		name   string
		offset int
		want   int
	}{
		{"start of file", 0, 0},
		{"inside the first line", 4, 0},
		{"the newline itself belongs to the line it ends", 9, 0},
		{"start of the blank line", 10, 1},
		{"start of the third line", 11, 2},
		{"inside the third line", 15, 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := index.LineOf(testCase.offset); got != testCase.want {
				t.Fatalf("LineOf(%d) = %d, want %d", testCase.offset, got, testCase.want)
			}
		})
	}
}

// TestLineIndexUTF16RoundTrip is the guard on the single subtlest thing in the
// whole adapter layer: a language server reports UTF-16 code units and never
// says so, because gopls does not advertise positionEncoding at all. Treating
// one as the other corrupts a position silently, and only on a line holding a
// character outside the basic plane.
func TestLineIndexUTF16RoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		source     string
		offset     int
		wantLine   int
		wantColumn int
	}{
		{"plain ascii", "func Add() {}\n", 5, 0, 5},
		{
			"after an accented letter, bytes and units diverge",
			"// café x\nfunc A(){}\n",
			11,
			0,
			10,
		},
		{"a two-byte letter counts as one unit", "var é = 1\nvar b = 2\n", 6, 0, 5},
		{"an emoji is a surrogate pair and counts as two", "// \U0001F600 x\nvar a = 1\n", 8, 0, 6},
		{"second line is measured from its own start", "var a = 1\nvar b = 2\n", 14, 1, 4},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			index := NewLineIndex([]byte(testCase.source))

			line := index.LineOf(testCase.offset)
			if line != testCase.wantLine {
				t.Fatalf("LineOf() = %d, want %d", line, testCase.wantLine)
			}

			column := index.UTF16Column(testCase.offset)
			if column != testCase.wantColumn {
				t.Fatalf("UTF16Column() = %d, want %d", column, testCase.wantColumn)
			}

			if back := index.ByteOffset(line, column); back != testCase.offset {
				t.Fatalf("ByteOffset(%d, %d) = %d, want %d", line, column, back, testCase.offset)
			}
		})
	}
}

func TestLineIndexByteOffsetOutOfRange(t *testing.T) {
	t.Parallel()

	index := NewLineIndex([]byte("var a = 1\nvar b = 2\n"))

	testCases := []struct {
		name string
		line int
		char int
		want int
	}{
		{"negative line", -1, 0, -1},
		{"line past the end", 9, 0, -1},
		{"a character past the line clamps to the line end, never the next line", 0, 99, 10},
		{"start of the second line", 1, 0, 10},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := index.ByteOffset(testCase.line, testCase.char); got != testCase.want {
				t.Fatalf(
					"ByteOffset(%d, %d) = %d, want %d",
					testCase.line,
					testCase.char,
					got,
					testCase.want,
				)
			}
		})
	}
}
