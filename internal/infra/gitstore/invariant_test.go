package gitstore

import (
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
)

func TestInvariantError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *InvariantError
		want string
	}{
		{
			"one diverging path",
			&InvariantError{Diverging: []string{"a.go"}},
			"final tree invariant failed: 1 paths diverge",
		},
		{
			"several diverging paths",
			&InvariantError{Diverging: []string{"a.go", "b.go"}},
			"final tree invariant failed: 2 paths diverge",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.err.Error(); got != testCase.want {
				t.Fatalf("Error() = %q, want %q", got, testCase.want)
			}

			if !errors.Is(testCase.err, apperr.ErrInvariant) {
				t.Fatalf("InvariantError does not unwrap to apperr.ErrInvariant")
			}
		})
	}
}
