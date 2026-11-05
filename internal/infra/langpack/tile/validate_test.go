package tile

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		source    string
		spans     []Span
		wantErr   error
		wantIndex int
	}{
		{"none", "", nil, nil, 0},
		{
			"ordered", "abcde",
			[]Span{{Start: 0, End: 2}, {Start: 2, End: 4}, {Start: 5, End: 5}},
			nil, 0,
		},
		{"whole source", "abcde", []Span{{Start: 0, End: 5}}, nil, 0},
		{"empty span in empty source", "", []Span{{Start: 0, End: 0}}, nil, 0},
		{"span in empty source", "", []Span{{Start: 0, End: 1}}, ErrSpanBounds, 0},
		{"inverted", "abcde", []Span{{Start: 0, End: 1}, {Start: 4, End: 3}}, ErrSpanInverted, 1},
		{"negative start", "abcde", []Span{{Start: -1, End: 2}}, ErrSpanBounds, 0},
		{"past the end", "abcde", []Span{{Start: 0, End: 1}, {Start: 1, End: 6}}, ErrSpanBounds, 1},
		{"overlap", "abcde", []Span{{Start: 0, End: 3}, {Start: 2, End: 4}}, ErrSpanOrder, 1},
		{"unordered", "abcde", []Span{{Start: 3, End: 4}, {Start: 0, End: 1}}, ErrSpanOrder, 1},
		{
			"first bad wins",
			"abcde",
			[]Span{{Start: 0, End: 9}, {Start: 3, End: 1}},
			ErrSpanBounds,
			0,
		},
		{"inverted beats bounds", "abcde", []Span{{Start: 6, End: 2}}, ErrSpanInverted, 0},
		{
			"bounds beats order", "abcde",
			[]Span{{Start: 0, End: 3}, {Start: 1, End: 9}},
			ErrSpanBounds, 1,
		},
		{
			"inverted beats order", "abcde",
			[]Span{{Start: 0, End: 3}, {Start: 2, End: 1}},
			ErrSpanInverted, 1,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := Validate([]byte(testCase.source), testCase.spans)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil {
				return
			}

			indexErr, isIndexErr := errors.AsType[*IndexError](err)
			if !isIndexErr || indexErr.Index != testCase.wantIndex {
				t.Fatalf("err = %#v, want index %d", err, testCase.wantIndex)
			}
		})
	}
}

func TestIndexError_Error(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *IndexError
		want string
	}{
		{"wrapped sentinel", &IndexError{Index: 2, Err: ErrSpanOrder}, ErrSpanOrder.Error()},
		{"nil cause", &IndexError{Index: 2}, "bad index"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.err.Error(); got != testCase.want {
				t.Fatalf("got %q, want %q", got, testCase.want)
			}

			if unwrapped := testCase.err.Unwrap(); unwrapped != testCase.err.Err {
				t.Fatalf("unwrap = %v, want %v", unwrapped, testCase.err.Err)
			}
		})
	}
}
