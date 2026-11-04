package e2e

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/router"
)

func TestConstructor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		raw       string
		wantErr   error
		wantGlobs []string
	}{
		{
			name:      "the router paths and the dirs are the globs",
			raw:       `{"type":"go-e2e","router":{"handle":["*.GET"],"paths":["api/"]},"dirs":["test/"]}`,
			wantGlobs: []string{"api/", "test/"},
		},
		{
			name:      "no router paths leaves the dirs",
			raw:       `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["test/"]}`,
			wantGlobs: []string{"test/"},
		},
		{
			name:    "an unknown key",
			raw:     `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["t/"],"x":1}`,
			wantErr: ErrConfig,
		},
		{
			name:    "a router that handles nothing",
			raw:     `{"type":"go-e2e","router":{"mount":["M*"]},"dirs":["t/"]}`,
			wantErr: router.ErrNoHandle,
		},
		{
			name:    "a router with an unknown key",
			raw:     `{"type":"go-e2e","router":{"handle":["*.GET"],"x":1},"dirs":["t/"]}`,
			wantErr: router.ErrConfig,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := Constructor("go-e2e", slog.Default())(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Constructor() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				return
			}

			rule, isRule := built.(*Rule)
			if !isRule || rule.Type() != "go-e2e" {
				t.Fatalf("Constructor() built %T", built)
			}

			if !reflect.DeepEqual(rule.Globs(), testCase.wantGlobs) {
				t.Fatalf("Globs() = %v, want %v", rule.Globs(), testCase.wantGlobs)
			}
		})
	}
}
