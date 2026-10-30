package format

import (
	"testing"
)

func TestStepNeeds(t *testing.T) {
	testCases := []struct {
		name    string
		wider   int
		tabs    int
		content string
		want    bool
	}{
		{"no limit runs always", 0, 4, "x", true},
		{"a short file is skipped", 10, 4, "abc\n", false},
		{"a long line runs", 10, 4, "abc\n01234567890\n", true},
		{"tabs count their width", 10, 4, "\t\t\tab\n", true},
		{"exactly the limit is skipped", 10, 4, "0123456789\n", false},
		{"a carriage return is not a column", 3, 4, "abc\r\n", false},
		{"an empty file is skipped", 10, 4, "", false},
		{"a tab advances to the next stop", 4, 4, "abc\t", false},
		{"a tab after a stop is a whole tab", 4, 4, "abcd\t", true},
		{"a tab inside a column pads to the stop", 5, 4, "ab\tc", false},
		{"a tab then text passes the stop", 4, 4, "ab\tc", true},
		{"wide runes count two columns", 5, 4, "\u65e5\u672c\u8a9e", true},
		{"wide runes within the limit are skipped", 6, 4, "\u65e5\u672c\u8a9e", false},
		{"fullwidth forms count two columns", 3, 4, "\uff21\uff22", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			step := tool{wider: testCase.wider, tabs: testCase.tabs}
			if got := step.needs([]byte(testCase.content)); got != testCase.want {
				t.Errorf("needs = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestColumns(t *testing.T) {
	testCases := []struct {
		name string
		line string
		want int
	}{
		{"ascii", "abc", 3},
		{"a rocket is wide", "\U0001F680", 2},
		{"a late emoji block is wide", "\U0001FA80", 2},
		{"a bmp emoji is wide", "\u2705\u2728\u274c\u2b50", 8},
		{"a bmp emoji in a gap range is wide", "\u231a\u23e9\u26a1\u2b55", 8},
		{"the playing card joker is wide", "\U0001F0CF", 2},
		{"a squared symbol is wide", "\U0001F18E\U0001F191", 4},
		{"the Yijing hexagrams are narrow", "\u4dc0\u4dff", 2},
		{"the char before the hexagrams is wide", "\u4dbf", 2},
		{"the char after the hexagrams is wide", "\u4e00", 2},
		{"a zero width joiner has no width", "a\u200db", 2},
		{"a variation selector has no width", "\u2764\ufe0f", 1},
		{"text presentation selector has no width", "a\ufe0e", 1},
		{"a combining mark has no width", "e\u0301", 1},
		{"an enclosing mark has no width", "a\u20dd", 1},
		{"a family sequence is the sum of its wide parts", "\U0001F468\u200d\U0001F469", 4},
		{"the orange circle is wide", "\U0001F7E0\U0001F7EB", 4},
		{"the squared ideographs are wide", "\U0001F200\U0001F251", 4},
		{"the char after the squared ideographs is narrow", "\U0001F252", 1},
		{"a medial jamo has no width", "\u1160\u11ff", 0},
		{"a tab pads to the next stop", "a\tb", 5},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := columns([]byte(testCase.line), 4); got != testCase.want {
				t.Errorf("columns = %d, want %d", got, testCase.want)
			}
		})
	}
}
