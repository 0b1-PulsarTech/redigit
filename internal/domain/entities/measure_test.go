package entities

import (
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
)

func TestMeasureValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		measure Measure
		wantErr error
	}{
		{
			"complete",
			Measure{
				Name:     "single directory",
				Value:    4,
				Total:    10,
				Excluded: []string{"prose commits"},
			},
			nil,
		},
		{
			"a measure that really looks at everything still says so",
			Measure{
				Name:     "declarations placed",
				Value:    10,
				Total:    10,
				Excluded: []string{"nothing"},
			},
			nil,
		},
		{
			"no name",
			Measure{Value: 1, Total: 2, Excluded: []string{"nothing"}},
			apperr.ErrIncomplete,
		},
		{
			"silent blind spot is the whole failure this type exists to stop",
			Measure{Name: "vet sweep", Value: 300, Total: 300},
			apperr.ErrIncomplete,
		},
		{
			"value above total",
			Measure{Name: "impossible", Value: 11, Total: 10, Excluded: []string{"nothing"}},
			apperr.ErrIncomplete,
		},
		{
			"negative total",
			Measure{Name: "negative", Value: 0, Total: -1, Excluded: []string{"nothing"}},
			apperr.ErrIncomplete,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.measure.Validate()
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestMeasurePercent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		measure Measure
		want    int
	}{
		{"half", Measure{Value: 5, Total: 10}, 50},
		{"none", Measure{Value: 0, Total: 10}, 0},
		{"all", Measure{Value: 10, Total: 10}, 100},
		{"empty total does not divide by zero", Measure{Value: 0, Total: 0}, 0},
		{"truncates rather than rounds", Measure{Value: 2, Total: 3}, 66},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.measure.Percent(); got != testCase.want {
				t.Fatalf("Percent() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestReportValidate(t *testing.T) {
	t.Parallel()

	good := Measure{Name: "trees match", Value: 1, Total: 1, Excluded: []string{"nothing"}}
	blind := Measure{Name: "grouping", Value: 1, Total: 1}

	testCases := []struct {
		name    string
		report  Report
		wantErr error
	}{
		{"empty report", Report{}, nil},
		{"both kinds sound", Report{Integrity: []Measure{good}, Shape: []Measure{good}}, nil},
		{
			"a blind shape measure fails even when integrity is sound",
			Report{Integrity: []Measure{good}, Shape: []Measure{blind}},
			apperr.ErrIncomplete,
		},
		{
			"a blind integrity measure fails too",
			Report{Integrity: []Measure{blind}, Shape: []Measure{good}},
			apperr.ErrIncomplete,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.report.Validate()
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
