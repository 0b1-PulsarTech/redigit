package shaping

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

type (
	namedPass struct{ name string }

	buildCase struct {
		name        string
		entries     []Entry
		raw         string
		wantNames   []string
		wantErr     error
		wantErrText string
	}
)

func (pass namedPass) Name() string { return pass.name }

func (namedPass) Apply(context.Context, *Draft) (Moves, error) { return nil, nil }

func namedConstructor() Constructor {
	return func(raw json.RawMessage) (Pass, error) {
		var spec struct {
			Type string `json:"type"`
			Tag  string `json:"tag"`
		}

		if err := DecodeStrict(raw, &spec); err != nil {
			return nil, err
		}

		return namedPass{spec.Type + spec.Tag}, nil
	}
}

func buildCases(boom error) []buildCase {
	named := namedConstructor()
	alpha := []Entry{{"alpha", named}}
	failing := func(json.RawMessage) (Pass, error) { return nil, boom }

	return []buildCase{
		{name: "absent section", entries: alpha},
		{name: "null section", entries: alpha, raw: "null"},
		{name: "empty list", entries: alpha, raw: "[]"},
		{
			name:      "built in listed order",
			entries:   []Entry{{"alpha", named}, {"beta", named}},
			raw:       `[{"type":"beta"},{"type":"alpha","tag":"x"}]`,
			wantNames: []string{"beta", "alphax"},
		},
		{
			name:        "not a list",
			entries:     alpha,
			raw:         `{"type":"alpha"}`,
			wantErr:     ErrMalformed,
			wantErrText: "passes: malformed pass: expected an array of objects",
		},
		{
			name:        "entry not an object",
			entries:     alpha,
			raw:         `["alpha"]`,
			wantErr:     ErrMalformed,
			wantErrText: "passes[0]: malformed pass: expected an object",
		},
		{
			name:        "missing type",
			entries:     alpha,
			raw:         `[{"tag":"x"}]`,
			wantErr:     ErrMissingType,
			wantErrText: "passes[0]: missing type",
		},
		{
			name:        "unknown type lists the known ones sorted",
			entries:     []Entry{{"beta", named}, {"alpha", named}},
			raw:         `[{"type":"alpha"},{"type":"nope"}]`,
			wantErr:     ErrUnknownType,
			wantErrText: `passes[1]: unknown type "nope" (known: alpha, beta)`,
		},
		{
			name:        "unknown type with nothing registered",
			raw:         `[{"type":"nope"}]`,
			wantErr:     ErrUnknownType,
			wantErrText: `passes[0]: unknown type "nope" (no pass types are registered)`,
		},
		{
			name:        "constructor error",
			entries:     []Entry{{"alpha", named}, {"bad", failing}},
			raw:         `[{"type":"alpha"},{"type":"bad"}]`,
			wantErr:     boom,
			wantErrText: "passes[1] (bad): boom",
		},
		{
			name:        "strict decoding rejects an unknown field",
			entries:     alpha,
			raw:         `[{"type":"alpha","tagg":"x"}]`,
			wantErr:     ErrMalformed,
			wantErrText: `passes[0] (alpha): malformed pass: json: unknown field "tagg"`,
		},
	}
}

func TestNewRegistry(t *testing.T) {
	t.Parallel()

	named := namedConstructor()

	testCases := []struct {
		name    string
		entries []Entry
		wantErr error
	}{
		{"none", nil, nil},
		{"distinct types", []Entry{{"alpha", named}, {"beta", named}}, nil},
		{"duplicate type", []Entry{{"alpha", named}, {"alpha", named}}, ErrDuplicateType},
		{"empty type", []Entry{{"", named}}, ErrMissingType},
		{"nil constructor", []Entry{{"alpha", nil}}, ErrMalformed},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewRegistry(testCase.entries...); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("NewRegistry() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestRegistryBuild(t *testing.T) {
	t.Parallel()

	for _, testCase := range buildCases(errors.New("boom")) {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			registry, err := NewRegistry(testCase.entries...)
			if err != nil {
				t.Fatalf("NewRegistry() = %v", err)
			}

			passes, err := registry.Build(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Build() error = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && err.Error() != testCase.wantErrText {
				t.Fatalf("Build() error text = %q, want %q", err.Error(), testCase.wantErrText)
			}

			got := make([]string, 0, len(passes))
			for _, pass := range passes {
				got = append(got, pass.Name())
			}

			if !slices.Equal(got, testCase.wantNames) {
				t.Fatalf("Build() names = %v, want %v", got, testCase.wantNames)
			}
		})
	}
}
