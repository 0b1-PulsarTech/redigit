package subjects

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func TestRunIsAUsageErrorUntilItsSliceArrives(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"some arguments", []string{"--repo", "."}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer

			err := Run(context.Background(), testCase.args, &stdout)
			if !errors.Is(err, write.ErrUsage) {
				t.Fatalf("Run() err = %v, want it to wrap ErrUsage", err)
			}

			if !strings.Contains(err.Error(), "arrives in slice 19") || stdout.Len() != 0 {
				t.Fatalf("Run() err = %v, stdout = %q", err, stdout.String())
			}
		})
	}
}
