package lineage

import "testing"

func TestRenameByText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		rename Rename
		want   bool
	}{
		{"file score only", Rename{FileScore: 90}, false},
		{"zero", Rename{}, false},
		{"text score", Rename{FileScore: 40, TextScore: 80}, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.rename.ByText(); got != testCase.want {
				t.Fatalf("ByText() = %v, want %v", got, testCase.want)
			}
		})
	}
}
