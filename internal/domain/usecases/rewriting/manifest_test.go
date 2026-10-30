package rewriting

import (
	"errors"
	"slices"
	"testing"
)

func TestOrderRows(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		rows    []Row
		want    []int
		wantErr error
	}{
		{
			"sorted by position",
			[]Row{{Position: 2}, {Position: 0}, {Position: 1}},
			[]int{0, 1, 2},
			nil,
		},
		{"empty", nil, []int{}, nil},
		{
			"duplicate position is refused",
			[]Row{{Position: 1}, {Position: 0}, {Position: 1}},
			nil,
			ErrManifest,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := OrderRows(testCase.rows)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("OrderRows() err = %v, want %v", err, testCase.wantErr)
			}

			positions := []int{}
			for _, row := range got {
				positions = append(positions, row.Position)
			}

			if testCase.wantErr == nil && !slices.Equal(positions, testCase.want) {
				t.Fatalf("OrderRows() positions = %v, want %v", positions, testCase.want)
			}
		})
	}
}
