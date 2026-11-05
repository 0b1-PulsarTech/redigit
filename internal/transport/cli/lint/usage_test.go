package lint

import (
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
)

func TestUsageSaysWhatItDoesNotDetectAndWhatTheNamesAre(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want []string
	}{
		{name: "red: the deferred names are given as printed", want: linting.Deferred()},
		{name: "red: --mode reaches only the cache key", want: []string{"cache key"}},
		{
			name: "red: the mismatches not detected are listed",
			want: []string{"not detected", "altered span", "altered unit", "build tags"},
		},
		{
			name: "red: --fail-over counts an unused declaration only past the stub gap",
			want: []string{"notCounted"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, want := range testCase.want {
				if !strings.Contains(Usage, want) {
					t.Fatalf("Usage does not say %q", want)
				}
			}
		})
	}
}
