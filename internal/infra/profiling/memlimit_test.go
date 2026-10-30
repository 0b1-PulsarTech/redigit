package profiling

import (
	"errors"
	"math"
	"testing"
)

func TestParseMemLimit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		text    string
		want    int64
		wantErr error
	}{
		{"plain bytes", "1024", 1024, nil},
		{"explicit bytes", "7B", 7, nil},
		{"kibibytes", "2KiB", 2 << 10, nil},
		{"mebibytes", "512MiB", 512 << 20, nil},
		{"gibibytes", "3GiB", 3 << 30, nil},
		{"tebibytes", "1TiB", 1 << 40, nil},
		{"surrounding space", "  4MiB ", 4 << 20, nil},
		{"zero", "0", 0, ErrMemLimit},
		{"zero with a unit", "0MiB", 0, ErrMemLimit},
		{"off", "off", math.MaxInt64, nil},
		{"empty", "", 0, ErrMemLimit},
		{"negative", "-1", 0, ErrMemLimit},
		{"unknown unit", "5XiB", 0, ErrMemLimit},
		{"decimal", "1.5GiB", 0, ErrMemLimit},
		{"unit only", "GiB", 0, ErrMemLimit},
		{"overflow", "9999999TiB", 0, ErrMemLimit},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMemLimit(testCase.text)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf(
					"ParseMemLimit(%q) err = %v, want %v",
					testCase.text,
					err,
					testCase.wantErr,
				)
			}

			if got != testCase.want {
				t.Fatalf("ParseMemLimit(%q) = %d, want %d", testCase.text, got, testCase.want)
			}
		})
	}
}
