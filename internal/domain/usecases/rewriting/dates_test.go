package rewriting

import (
	"errors"
	"slices"
	"testing"
)

func TestSampleDates(t *testing.T) {
	t.Parallel()

	tens := []Stamp{
		{50, "+0000"},
		{0, "+0000"},
		{10, "+0000"},
		{20, "+0000"},
		{30, "+0000"},
		{40, "+0000"},
	}

	testCases := []struct {
		name    string
		stamps  []Stamp
		count   int
		want    []Stamp
		wantErr error
	}{
		{"count 1 takes the last stamp", tens, 1, []Stamp{{50, "+0000"}}, nil},
		{"count zero is empty", tens, 0, nil, nil},
		{
			"count equals n keeps every stamp",
			tens[:3],
			3,
			[]Stamp{{0, "+0000"}, {10, "+0000"}, {50, "+0000"}},
			nil,
		},
		{
			"half ties round to even", tens, 3,
			[]Stamp{{0, "+0000"}, {20, "+0000"}, {50, "+0000"}},
			nil,
		},
		{
			"low tie rounds down and duplicates are bumped",
			[]Stamp{{100, "+0000"}, {200, "+0000"}},
			3,
			[]Stamp{{100, "+0000"}, {101, "+0000"}, {200, "+0000"}},
			nil,
		},
		{
			"each stamp keeps its own zone when bumped",
			[]Stamp{{100, "+0100"}, {100, "-0300"}},
			2,
			[]Stamp{{100, "+0100"}, {101, "-0300"}},
			nil,
		},
		{
			"equal stamps sort by zone",
			[]Stamp{{100, "-0300"}, {100, "+0100"}},
			1,
			[]Stamp{{100, "-0300"}},
			nil,
		},
		{"empty stamps are an error", nil, 2, nil, ErrNoDates},
		{"a negative stamp is refused", []Stamp{{-1, "+0"}, {5, "+0"}}, 2, nil, ErrNegativeStamp},
		{"a lone pre-1970 stamp is refused", []Stamp{{-100, "+0000"}}, 1, nil, ErrNegativeStamp},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			input := slices.Clone(testCase.stamps)

			got, err := SampleDates(testCase.stamps, testCase.count)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("SampleDates() err = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("SampleDates() = %v, want %v", got, testCase.want)
			}

			if !slices.Equal(input, testCase.stamps) {
				t.Fatalf("SampleDates() mutated its input")
			}
		})
	}
}
