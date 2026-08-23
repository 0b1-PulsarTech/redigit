package chunk

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestLocationAt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		offset int
		want   entities.Location
	}{
		{"start of file", "abc\ndef\n", 0, entities.Location{File: "f", Line: 0, Char: 0}},
		{"mid first line", "abc\ndef\n", 1, entities.Location{File: "f", Line: 0, Char: 1}},
		{"start of second line", "abc\ndef\n", 4, entities.Location{File: "f", Line: 1, Char: 0}},
		{
			"non-ASCII counts UTF-16 units, not bytes",
			"café\ndef\n",
			// offset 5 is right after "café" (3 ASCII bytes + a 2-byte
			// e-acute = 5 bytes), which is 4 UTF-16 units, not 5.
			5,
			entities.Location{File: "f", Line: 0, Char: 4},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)
			lines := splitLines(source)

			got := locationAt("f", source, lines, testCase.offset)
			if got != testCase.want {
				t.Fatalf("locationAt() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestUTF16Units(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string
		want int
	}{
		{"ascii", "abc", 3},
		{"empty", "", 0},
		{"two-byte rune", "é", 1},
		{"surrogate pair emoji", "\U0001f600", 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := utf16Units([]byte(testCase.text)); got != testCase.want {
				t.Fatalf("utf16Units(%q) = %d, want %d", testCase.text, got, testCase.want)
			}
		})
	}
}
