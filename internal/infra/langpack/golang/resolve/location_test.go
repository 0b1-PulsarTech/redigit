package resolve

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestTextFileOffset(t *testing.T) {
	t.Parallel()

	file := newTextFile([]byte("ab\n\U0001F600x\n\nlast"))

	testCases := []struct {
		name   string
		at     entities.Location
		want   int
		wantOK bool
	}{
		{"the start", entities.Location{}, 0, true},
		{"inside an ascii line", entities.Location{Char: 1}, 1, true},
		{"the end of a line", entities.Location{Char: 2}, 2, true},
		{"after an emoji, two UTF-16 units", entities.Location{Line: 1, Char: 2}, 7, true},
		{"between the halves of an emoji", entities.Location{Line: 1, Char: 1}, 0, false},
		{"after the emoji and its neighbour", entities.Location{Line: 1, Char: 3}, 8, true},
		{"an empty line", entities.Location{Line: 2}, 9, true},
		{"the last line without a newline", entities.Location{Line: 3, Char: 4}, 14, true},
		{"a column past the line", entities.Location{Char: 9}, 0, false},
		{"a line past the file", entities.Location{Line: 9}, 0, false},
		{"a negative line", entities.Location{Line: -1}, 0, false},
		{"a negative column", entities.Location{Char: -1}, 0, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, ok := file.offset(testCase.at)
			if ok != testCase.wantOK || (ok && got != testCase.want) {
				t.Fatalf(
					"offset(%v) = %d, %v, want %d, %v",
					testCase.at,
					got,
					ok,
					testCase.want,
					testCase.wantOK,
				)
			}
		})
	}
}

func TestTextFileOffsetStopsAtTheLineEnd(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		text   string
		at     entities.Location
		want   int
		wantOK bool
	}{
		{"the newline of an LF line", "ab\ncd\n", entities.Location{Char: 2}, 2, true},
		{"one past the newline of an LF line", "ab\ncd\n", entities.Location{Char: 3}, 0, false},
		{"the start of the next LF line", "ab\ncd\n", entities.Location{Line: 1}, 3, true},
		{"the carriage return of a CRLF line", "ab\r\ncd\r\n", entities.Location{Char: 2}, 2, true},
		{"between the CRLF pair", "ab\r\ncd\r\n", entities.Location{Char: 3}, 0, false},
		{"one past a CRLF line", "ab\r\ncd\r\n", entities.Location{Char: 4}, 0, false},
		{"the start of the next CRLF line", "ab\r\ncd\r\n", entities.Location{Line: 1}, 4, true},
		{"after the emoji on a CRLF line", "\U0001F600x\r\n", entities.Location{Char: 3}, 5, true},
		{"the end of a last line", "ab\r\ncd", entities.Location{Line: 1, Char: 2}, 6, true},
		{"past the end of a last line", "ab\r\ncd", entities.Location{Line: 1, Char: 3}, 0, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, ok := newTextFile([]byte(testCase.text)).offset(testCase.at)
			if ok != testCase.wantOK || (ok && got != testCase.want) {
				t.Fatalf(
					"offset(%v) = %d, %v, want %d, %v", testCase.at, got, ok, testCase.want,
					testCase.wantOK,
				)
			}
		})
	}
}
