package typed

import (
	"context"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestTypedSourceKeepsAPartialPassAsData(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		files      map[string]string
		wantBroken bool
	}{
		{
			"every package checks",
			map[string]string{"a.go": "package fixture\n\nfunc One() {}\n"},
			false,
		},
		{
			"a package does not type-check",
			map[string]string{"a.go": "package fixture\n\nvar Bad int = \"text\"\n"},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, testCase.files)
			source := NewSource([]string{"integration"})

			if got := source.Shortfalls(); len(got) != 0 {
				t.Fatalf("Shortfalls() = %+v before any load", got)
			}

			if _, err := source.Facts(context.Background(), snap, decls); err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			got := source.Shortfalls()
			if (len(got) > 0) != testCase.wantBroken {
				t.Fatalf("Shortfalls() = %+v, want broken = %v", got, testCase.wantBroken)
			}

			if testCase.wantBroken &&
				(got[0].Broken < 1 || got[0].Of < got[0].Broken ||
					!slices.Equal(got[0].Tags, []string{"integration"})) {
				t.Fatalf("Shortfalls() = %+v, want the broken count, total and tags", got)
			}
		})
	}
}
