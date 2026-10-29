package batch

import (
	"strings"
	"testing"
)

func TestTailBuffer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		writes []string
		want   string
	}{
		{name: "empty"},
		{
			name:   "short is kept whole and trimmed",
			writes: []string{"fatal: ", "bad\n"},
			want:   "fatal: bad",
		},
		{
			name:   "long keeps only the last bytes",
			writes: []string{strings.Repeat("x", stderrLimit), "tail"},
			want:   strings.Repeat("x", stderrLimit-len("tail")) + "tail",
		},
		{
			name:   "one oversized write keeps its end",
			writes: []string{strings.Repeat("y", 3*stderrLimit)},
			want:   strings.Repeat("y", stderrLimit),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var tail tailBuffer

			for _, chunk := range testCase.writes {
				if written, err := tail.Write([]byte(chunk)); err != nil || written != len(chunk) {
					t.Fatalf("Write() = %d, %v", written, err)
				}
			}

			if got := tail.String(); got != testCase.want {
				t.Fatalf("String() len %d, want len %d", len(got), len(testCase.want))
			}
		})
	}
}
