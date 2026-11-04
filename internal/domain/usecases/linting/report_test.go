package linting

import (
	"slices"
	"testing"
)

func TestDeferredNamesTheFiveUnmeasured(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want []string
	}{
		{
			name: "the five measures that wait for the plan pipeline, in order",
			want: []string{
				"errcheck_pairs_split", "shells_without_tail", "shells_born_empty",
				"unused_at_birth", "born_as_stubs_gap_over_20_rows",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Deferred(); !slices.Equal(got, testCase.want) {
				t.Fatalf("Deferred() = %v, want %v", got, testCase.want)
			}
		})
	}
}
