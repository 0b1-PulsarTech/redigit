package lineageconf

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

const (
	firstRev = "3d2d883000000000000000000000000000000000"

	stoppedConfig = `{"write": {"identity": {"name": "n"}}, "lineage": {"policy": "stop", "anchors": [
		{"rev": "3d2d883", "kind": "fix"}, {"rev": "0aab4c6", "kind": "perf"}]}}`
)

type decideCase struct {
	name         string
	config       string
	decisions    map[string]anchoring.Policy
	rest         anchoring.Policy
	wantPolicy   anchoring.Policy
	wantAnchors  []anchoring.Spec
	wantOtherKey string
}

func checkDecide(t *testing.T, testCases []decideCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out, err := Decide([]byte(testCase.config), testCase.decisions, testCase.rest)
			if err != nil {
				t.Fatalf("Decide() = %v", err)
			}

			var file map[string]json.RawMessage
			if err = json.Unmarshal(out, &file); err != nil {
				t.Fatalf("Decide() wrote %s: %v", out, err)
			}

			section, err := Decode(file["lineage"])
			if err != nil {
				t.Fatalf("Decode() = %v", err)
			}

			if section.Policy != testCase.wantPolicy ||
				!slices.Equal(section.Anchors, testCase.wantAnchors) {
				t.Fatalf("section = %+v, want %q %v",
					section, testCase.wantPolicy, testCase.wantAnchors)
			}

			if _, isKept := file[testCase.wantOtherKey]; testCase.wantOtherKey != "" && !isKept {
				t.Fatalf("Decide() dropped the %q key: %s", testCase.wantOtherKey, out)
			}
		})
	}
}

func TestDecideDecisions(t *testing.T) {
	t.Parallel()

	checkDecide(t, []decideCase{
		{
			name:       "one fold leaves the other anchor and the section as they were",
			config:     stoppedConfig,
			decisions:  map[string]anchoring.Policy{firstRev: anchoring.PolicyFold},
			wantPolicy: anchoring.PolicyStop,
			wantAnchors: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix", Policy: anchoring.PolicyFold},
				{Rev: "0aab4c6", Kind: "perf"},
			},
			wantOtherKey: "write",
		},
		{
			name: "one wip is written on its own anchor", config: stoppedConfig,
			decisions:  map[string]anchoring.Policy{"0aab4c6": anchoring.PolicyWIP},
			wantPolicy: anchoring.PolicyStop,
			wantAnchors: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix"},
				{Rev: "0aab4c6", Kind: "perf", Policy: anchoring.PolicyWIP},
			},
		},
	})
}

func TestDecideRest(t *testing.T) {
	t.Parallel()

	const perAnchorConfig = `{"lineage": {"policy": "fold", "anchors": [
		{"rev": "3d2d883", "kind": "fix", "policy": "stop"},
		{"rev": "0aab4c6", "kind": "perf", "policy": "fold"}]}}`

	checkDecide(t, []decideCase{
		{
			name: "rest fold replaces a section stop", config: stoppedConfig,
			rest: anchoring.PolicyFold, wantPolicy: anchoring.PolicyFold,
			wantAnchors: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix"}, {Rev: "0aab4c6", Kind: "perf"},
			},
		},
		{
			name: "a decision wins over the rest", config: stoppedConfig, rest: anchoring.PolicyWIP,
			decisions:  map[string]anchoring.Policy{firstRev: anchoring.PolicyFold},
			wantPolicy: anchoring.PolicyWIP,
			wantAnchors: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix", Policy: anchoring.PolicyFold},
				{Rev: "0aab4c6", Kind: "perf"},
			},
		},
		{
			name:       "rest wip replaces an anchor stop and spares an anchor fold",
			config:     perAnchorConfig,
			rest:       anchoring.PolicyWIP,
			wantPolicy: anchoring.PolicyFold,
			wantAnchors: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix", Policy: anchoring.PolicyWIP},
				{Rev: "0aab4c6", Kind: "perf", Policy: anchoring.PolicyFold},
			},
		},
	})
}

func TestDecideRefusals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config string
	}{
		{name: "a config that is not json", config: `{`},
		{name: "a config that is not an object", config: `[1]`},
		{name: "a config with no lineage section", config: `{"write": {}}`},
		{name: "a lineage section that is invalid", config: `{"lineage": {"policy": "later"}}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Decide([]byte(testCase.config), nil, anchoring.PolicyFold); err == nil {
				t.Fatal("Decide() = nil, want a refusal")
			}
		})
	}
}

func TestDecideSize(t *testing.T) {
	t.Parallel()

	const limit = 1 << 20

	padded := func(pad int) []byte {
		return []byte(`{"pad":"` + strings.Repeat("x", pad) + `","lineage":{"policy":"stop"}}`)
	}

	base, err := Decide(padded(0), nil, anchoring.PolicyFold)
	if err != nil {
		t.Fatalf("Decide() = %v", err)
	}

	many := `{"list":[` + strings.Repeat(`"ab",`, 150000) + `"ab"],"lineage":{"policy":"stop"}}`

	testCases := []struct {
		name    string
		config  []byte
		wantErr error
	}{
		{name: "a derived config of exactly the limit", config: padded(limit - len(base))},
		{
			name:    "a derived config one byte over the limit",
			config:  padded(limit - len(base) + 1),
			wantErr: rules.ErrTooLarge,
		},
		{name: "a config that only indentation would push over the limit", config: []byte(many)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out, decideErr := Decide(testCase.config, nil, anchoring.PolicyFold)
			if !errors.Is(decideErr, testCase.wantErr) {
				t.Fatalf("Decide() = %v, want %v", decideErr, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				return
			}

			path := filepath.Join(t.TempDir(), "derived.json")
			if writeErr := os.WriteFile(path, out, 0o600); writeErr != nil {
				t.Fatal(writeErr)
			}

			if _, readErr := rules.ReadBounded(path); readErr != nil || len(out) > limit {
				t.Fatalf("the derived config is %d bytes and reads as %v", len(out), readErr)
			}
		})
	}
}
