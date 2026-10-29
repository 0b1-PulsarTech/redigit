package parity

import (
	"errors"
	"slices"
	"testing"
)

func rowOf(position int, keys ...string) Row {
	row := Row{Position: position}
	for _, key := range keys {
		row.Chunks = append(row.Chunks, Chunk{Key: key})
	}

	return row
}

func TestRowSetEquality(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		left, right  []Row
		wantValue    int
		wantTotal    int
		wantDiffers  []int
		wantFraction float64
		wantDefined  bool
	}{
		{
			name:      "identical regardless of chunk order",
			left:      []Row{rowOf(0, "a", "b"), rowOf(1, "c")},
			right:     []Row{rowOf(0, "b", "a"), rowOf(1, "c")},
			wantValue: 2, wantTotal: 2, wantDiffers: []int{}, wantFraction: 1, wantDefined: true,
		},
		{
			name:      "one position differs",
			left:      []Row{rowOf(0, "a"), rowOf(1, "c")},
			right:     []Row{rowOf(0, "a"), rowOf(1, "d")},
			wantValue: 1, wantTotal: 2, wantDiffers: []int{1}, wantFraction: 0.5, wantDefined: true,
		},
		{
			name:      "position on one side only",
			left:      []Row{rowOf(0, "a")},
			right:     []Row{rowOf(0, "a"), rowOf(1, "b")},
			wantValue: 1, wantTotal: 2, wantDiffers: []int{1}, wantFraction: 0.5, wantDefined: true,
		},
		{name: "both empty", wantDiffers: []int{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := RowSetEquality(testCase.left, testCase.right)
			if err != nil {
				t.Fatalf("RowSetEquality: %v", err)
			}

			if got.Measure.Value != testCase.wantValue || got.Measure.Total != testCase.wantTotal {
				t.Fatalf("measure = %+v", got.Measure)
			}

			if !slices.Equal(got.Differs, testCase.wantDiffers) {
				t.Fatalf("differs = %v, want %v", got.Differs, testCase.wantDiffers)
			}

			fraction, defined := got.Fraction()
			if fraction != testCase.wantFraction || defined != testCase.wantDefined {
				t.Fatalf("fraction = %v defined = %v", fraction, defined)
			}

			if err := got.Measure.Validate(); err != nil {
				t.Fatalf("measure invalid: %v", err)
			}
		})
	}
}

func TestRowSetEqualityRejectsDuplicatePosition(t *testing.T) {
	t.Parallel()

	good := []Row{rowOf(0, "a")}
	dup := []Row{rowOf(0, "a"), rowOf(0, "b")}

	testCases := []struct {
		name        string
		left, right []Row
	}{
		{"left", dup, good},
		{"right", good, dup},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := RowSetEquality(testCase.left, testCase.right)
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("err = %v, want ErrMalformed", err)
			}
		})
	}
}
