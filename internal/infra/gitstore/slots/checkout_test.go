package slots

import (
	"slices"
	"testing"
)

func TestReadTreeArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		prev string
		next string
		want []string
	}{
		{
			"first checkout reads one tree",
			"", "t2",
			[]string{"read-tree", "-m", "-u", "--no-sparse-checkout", "t2"},
		},
		{
			"later checkouts merge two trees",
			"t1", "t2",
			[]string{"read-tree", "-m", "-u", "--no-sparse-checkout", "t1", "t2"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := readTreeArgs(
				testCase.prev,
				testCase.next,
			); !slices.Equal(
				got,
				testCase.want,
			) {
				t.Fatalf("args = %v, want %v", got, testCase.want)
			}
		})
	}
}
