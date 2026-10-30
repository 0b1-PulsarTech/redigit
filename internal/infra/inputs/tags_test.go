package inputs

import (
	"errors"
	"maps"
	"testing"
)

func TestSplitTags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		tags string
		want []string
	}{
		{"empty string yields none", "", nil},
		{"whitespace only yields none", "   ", nil},
		{"one tag", "a", []string{"a"}},
		{"two tags", "a,b", []string{"a", "b"}},
		{"blank entries between commas are dropped", "a,,b", []string{"a", "b"}},
		{"surrounding whitespace is trimmed", " a , b ", []string{"a", "b"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := SplitTags(testCase.tags)

			if len(got) != len(testCase.want) {
				t.Fatalf("SplitTags(%q) = %v, want %v", testCase.tags, got, testCase.want)
			}

			for idx, tag := range testCase.want {
				if got[idx] != tag {
					t.Fatalf("SplitTags(%q) = %v, want %v", testCase.tags, got, testCase.want)
				}
			}
		})
	}
}

func TestAddVirtual(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		prior   map[string]string
		id      string
		path    string
		want    map[string]string
		wantErr error
	}{
		{"a pair", map[string]string{}, "a", "x", map[string]string{"a": "x"}, nil},
		{"trimmed", map[string]string{}, " a ", " x ", map[string]string{"a": "x"}, nil},
		{"an empty id", map[string]string{}, "", "x", map[string]string{}, ErrVirtual},
		{"a blank path", map[string]string{}, "a", "  ", map[string]string{}, ErrVirtual},
		{
			"an id already listed",
			map[string]string{"a": "x"},
			" a", "y",
			map[string]string{"a": "x"},
			ErrDuplicateVirtual,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := AddVirtual(testCase.prior, testCase.id, testCase.path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("AddVirtual() err = %v, want %v", err, testCase.wantErr)
			}

			if !maps.Equal(testCase.prior, testCase.want) {
				t.Fatalf("virtual = %v, want %v", testCase.prior, testCase.want)
			}
		})
	}
}
