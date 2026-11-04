package e2e

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/router"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		raw     string
		want    Config
		wantErr error
	}{
		{
			name: "a router and dirs",
			raw:  `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["test/"]}`,
			want: Config{Router: json.RawMessage(`{"handle":["*.GET"]}`), Dirs: []string{"test/"}},
		},
		{
			name:    "sinks have no meaning here",
			raw:     `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["test/"],"sinks":[]}`,
			wantErr: ErrConfig,
		},
		{
			name:    "sweeps have no meaning here",
			raw:     `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["test/"],"sweeps":[]}`,
			wantErr: ErrConfig,
		},
		{
			name:    "no router",
			raw:     `{"type":"go-e2e","dirs":["test/"]}`,
			wantErr: ErrNoRouter,
		},
		{
			name:    "a dir with a star is refused",
			raw:     `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["t*/"]}`,
			wantErr: ErrBadDir,
		},
		{
			name:    "a dir with a bracket is refused",
			raw:     `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["t[ab]/"]}`,
			wantErr: ErrBadDir,
		},
		{
			name: "requests are kept",
			raw:  `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["t/"],"requests":["*.Get"]}`,
			want: Config{
				Router:   json.RawMessage(`{"handle":["*.GET"]}`),
				Dirs:     []string{"t/"},
				Requests: []string{"*.Get"},
			},
		},
		{
			name:    "a malformed requests pattern",
			raw:     `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["t/"],"requests":["["]}`,
			wantErr: router.ErrBadPattern,
		},
		{
			name:    "no dirs",
			raw:     `{"type":"go-e2e","router":{"handle":["*.GET"]}}`,
			wantErr: ErrNoDirs,
		},
		{name: "not json", raw: `[`, wantErr: ErrConfig},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := decode(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("decode() err = %v, want %v", err, testCase.wantErr)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("decode() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
