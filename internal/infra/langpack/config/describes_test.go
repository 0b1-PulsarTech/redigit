package config

import (
	"testing"
)

func TestIsProse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		file string
		want bool
	}{
		{"docs/specs/campaigns.md", true},
		{"modules/wabapi/README.md", true},
		{"docs/SERVICE_FLOW.mmd", true},
		{"notes/design.txt", true},
		{"docs/local/.gitignore", true},
		{"modules/x/doc/guide.yml", true},
		{"modules/x/config.yml", false},
		{"modules/x/documents/a.bin", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.file, func(t *testing.T) {
			t.Parallel()

			if got := isProse(testCase.file); got != testCase.want {
				t.Errorf("isProse(%q) = %v, want %v", testCase.file, got, testCase.want)
			}
		})
	}
}
