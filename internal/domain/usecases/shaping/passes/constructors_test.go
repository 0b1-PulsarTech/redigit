package passes

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

func TestEntries(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		raw      string
		wantName string
		wantErr  error
	}{
		{"tidy by its type", `[{"type":"tidy-test-after"}]`, "tidy-test-after", nil},
		{"unknown field", `[{"type":"tidy-test-after","nope":1}]`, "", shaping.ErrMalformed},
		{"unknown type", `[{"type":"nope"}]`, "", shaping.ErrUnknownType},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			registry, err := shaping.NewRegistry(Entries()...)
			if err != nil {
				t.Fatalf("NewRegistry() = %v", err)
			}

			built, err := registry.Build(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Build() error = %v, want %v", err, testCase.wantErr)
			}

			if err == nil && (len(built) != 1 || built[0].Name() != testCase.wantName) {
				t.Fatalf("Build() = %v, want one %s", built, testCase.wantName)
			}
		})
	}
}
