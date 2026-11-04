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
		{
			name:     "tidy by its type",
			raw:      `[{"type":"tidy-test-after"}]`,
			wantName: "tidy-test-after",
		},
		{name: "cap by its type", raw: `[{"type":"finish-cap"}]`, wantName: "finish-cap"},
		{
			name:    "cap unknown field",
			raw:     `[{"type":"finish-cap","nope":1}]`,
			wantErr: shaping.ErrMalformed,
		},
		{
			name:    "unknown field",
			raw:     `[{"type":"tidy-test-after","nope":1}]`,
			wantErr: shaping.ErrMalformed,
		},
		{name: "unknown type", raw: `[{"type":"nope"}]`, wantErr: shaping.ErrUnknownType},
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
