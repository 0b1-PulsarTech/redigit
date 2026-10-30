//go:build !unix

package format

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
	"testing"
)

func TestArgTooLong(t *testing.T) {
	const errFilenameExcedRange = syscall.Errno(206)

	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{"the command line past the limit", errFilenameExcedRange, true},
		{"the limit wrapped in a path error", &fs.PathError{Err: errFilenameExcedRange}, true},
		{"the limit wrapped by the runner", fmt.Errorf("run: %w", errFilenameExcedRange), true},
		{"an unrelated errno", syscall.Errno(2), false},
		{"a plain error", errors.New("boom"), false},
		{"no error", nil, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := argTooLong(testCase.err); got != testCase.want {
				t.Errorf("argTooLong = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestArgBudgetValue(t *testing.T) {
	const want, cmdExeLimit = 7500, 8191

	if argBudget != want || argBudget >= cmdExeLimit {
		t.Errorf("argBudget = %d, want %d, below the cmd.exe limit %d", argBudget, want, cmdExeLimit)
	}
}
