package chunk

import (
	"testing"
)

// TestRegistryWholeFileIsLast checks the one invariant the Registry exists
// to guarantee: WholeFile claims every path, so it must sit after every more
// specific chunker or those chunkers would never be tried.
func TestRegistryWholeFileIsLast(t *testing.T) {
	t.Parallel()

	if len(Registry) == 0 {
		t.Fatal("Registry is empty")
	}

	if _, ok := Registry[len(Registry)-1].(WholeFile); !ok {
		t.Fatalf("Registry's last entry = %T, want WholeFile", Registry[len(Registry)-1])
	}

	for _, chunker := range Registry[:len(Registry)-1] {
		if _, ok := chunker.(WholeFile); ok {
			t.Fatalf("WholeFile appears before the end of Registry")
		}
	}
}

func TestRegistryHandlesEveryPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
	}{
		{"a sqlc query file", "database/queries/lead.sql"},
		{"a go.mod", "modules/lead/go.mod"},
		{"a go.work", "go.work"},
		{"an unrelated file", "docs/PLANO.md"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, chunker := range Registry {
				if chunker.Handles(testCase.path) {
					return
				}
			}

			t.Fatalf("no chunker in Registry handles %q", testCase.path)
		})
	}
}
