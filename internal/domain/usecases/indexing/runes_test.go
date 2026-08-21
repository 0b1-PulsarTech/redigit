package indexing

import "testing"

func TestRuneSizeAt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		offset int
		want   int
	}{
		{"ascii", "abc", 0, 1},
		{"two-byte letter", "é", 0, 2},
		{"emoji", "\U0001F600", 0, 4},
		{"an invalid byte advances by one rather than stalling", "\xff", 0, 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := runeSizeAt([]byte(testCase.source), testCase.offset); got != testCase.want {
				t.Fatalf("runeSizeAt() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestUTF16Units(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		encoded string
		want    int
	}{
		{"empty", "", 0},
		{"ascii", "abc", 3},
		{"a two-byte letter is still one unit", "é", 1},
		{"an emoji is a surrogate pair", "\U0001F600", 2},
		{"mixed", "a\U0001F600é", 4},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := utf16Units([]byte(testCase.encoded)); got != testCase.want {
				t.Fatalf("utf16Units(%q) = %d, want %d", testCase.encoded, got, testCase.want)
			}
		})
	}
}
