package bootstrap

import (
	"errors"
	"slices"
	"testing"
)

func TestClosersCloseInReverseAndJoinEveryError(t *testing.T) {
	t.Parallel()

	errFirst := errors.New("first would not close")
	errLast := errors.New("last would not close")

	testCases := []struct {
		name      string
		build     func(order *[]string) closers
		wantOrder []string
		wantErrs  []error
	}{
		{name: "none held", build: func(*[]string) closers { return nil }},
		{
			name: "closed last to first",
			build: func(order *[]string) closers {
				return closers{orderedCloser{"a", order}, orderedCloser{"b", order}}
			},
			wantOrder: []string{"b", "a"},
		},
		{
			name: "one failure does not stop the rest",
			build: func(order *[]string) closers {
				return closers{
					failingCloser{orderedCloser{"a", order}, errFirst},
					orderedCloser{"b", order},
					failingCloser{orderedCloser{"c", order}, errLast},
				}
			},
			wantOrder: []string{"c", "b", "a"},
			wantErrs:  []error{errFirst, errLast},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var order []string

			err := testCase.build(&order).Close(t.Context())

			if !slices.Equal(order, testCase.wantOrder) {
				t.Fatalf("Close() stopped %v, want %v", order, testCase.wantOrder)
			}

			for _, target := range testCase.wantErrs {
				if !errors.Is(err, target) {
					t.Fatalf("Close() = %v, want %v wrapped", err, target)
				}
			}

			if len(testCase.wantErrs) == 0 && err != nil {
				t.Fatalf("Close() = %v, want nil", err)
			}
		})
	}
}
