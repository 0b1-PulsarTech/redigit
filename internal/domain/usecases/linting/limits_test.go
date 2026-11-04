package linting

import "testing"

func TestDefaultLimits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want Limits
	}{
		{
			name: "the finish-cap numbers of the original scanner",
			want: Limits{SourceAdded: 900, GeneratedMixed: 900, RowAdded: 850},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := DefaultLimits(); got != testCase.want {
				t.Fatalf("DefaultLimits() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
