package parity

import (
	"math"
	"testing"
)

func TestKendallTau(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		left, right []Row
		wantTau     float64
		wantDefined bool
		wantCommon  int
		wantValue   int
		wantTotal   int
	}{
		{
			name:    "same order",
			left:    []Row{rowOf(0, "a"), rowOf(1, "b"), rowOf(2, "c")},
			right:   []Row{rowOf(5, "a"), rowOf(6, "b"), rowOf(7, "c")},
			wantTau: 1, wantDefined: true, wantCommon: 3, wantValue: 3, wantTotal: 3,
		},
		{
			name:    "reversed",
			left:    []Row{rowOf(0, "a"), rowOf(1, "b"), rowOf(2, "c")},
			right:   []Row{rowOf(0, "c"), rowOf(1, "b"), rowOf(2, "a")},
			wantTau: -1, wantDefined: true, wantCommon: 3, wantValue: 0, wantTotal: 3,
		},
		{
			name:    "one swap",
			left:    []Row{rowOf(0, "a"), rowOf(1, "b"), rowOf(2, "c")},
			right:   []Row{rowOf(0, "b"), rowOf(1, "a"), rowOf(2, "c")},
			wantTau: 1.0 / 3, wantDefined: true, wantCommon: 3, wantValue: 2, wantTotal: 3,
		},
		{
			name:    "keys on one side only are ignored",
			left:    []Row{rowOf(0, "a", "x"), rowOf(1, "b")},
			right:   []Row{rowOf(0, "a"), rowOf(1, "b", "y")},
			wantTau: 1, wantDefined: true, wantCommon: 2, wantValue: 1, wantTotal: 1,
		},
		{
			name:    "ties inside a commit use tau-b",
			left:    []Row{rowOf(0, "a", "b"), rowOf(1, "c")},
			right:   []Row{rowOf(0, "a"), rowOf(1, "b", "c")},
			wantTau: 0.5, wantDefined: true, wantCommon: 3, wantValue: 1, wantTotal: 3,
		},
		{
			name:    "ties on both sides need the secondary sort key",
			left:    []Row{rowOf(0, "a", "b"), rowOf(1, "c")},
			right:   []Row{rowOf(0, "b"), rowOf(1, "a"), rowOf(2, "c")},
			wantTau: 2 / math.Sqrt(6), wantDefined: true, wantCommon: 3, wantValue: 2, wantTotal: 3,
		},
		{name: "nothing in common"},
		{
			name:       "one common key",
			left:       []Row{rowOf(0, "a")},
			right:      []Row{rowOf(0, "a")},
			wantCommon: 1,
		},
		{
			name:       "one side all ties",
			left:       []Row{rowOf(0, "a", "b")},
			right:      []Row{rowOf(0, "a"), rowOf(1, "b")},
			wantCommon: 2, wantTotal: 1,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := KendallTau(testCase.left, testCase.right)
			if math.Abs(got.Tau-testCase.wantTau) > 1e-9 || got.Common != testCase.wantCommon ||
				got.Defined != testCase.wantDefined {
				t.Fatalf("tau = %v common = %d", got.Tau, got.Common)
			}

			if got.Measure.Value != testCase.wantValue || got.Measure.Total != testCase.wantTotal {
				t.Fatalf("measure = %+v", got.Measure)
			}

			if err := got.Measure.Validate(); err != nil {
				t.Fatalf("measure invalid: %v", err)
			}
		})
	}
}

func TestCountInversions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		values []int
		want   int
	}{
		{"empty", nil, 0},
		{"sorted", []int{1, 2, 3}, 0},
		{"descending", []int{3, 2, 1}, 3},
		{"equal values are not inversions", []int{2, 2, 1}, 2},
		{"mixed", []int{2, 4, 1, 3, 5}, 3},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := countInversions(
				testCase.values,
				make([]int, len(testCase.values)),
			); got != testCase.want {
				t.Fatalf("inversions = %d, want %d", got, testCase.want)
			}
		})
	}
}
