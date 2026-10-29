package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type fakeRule struct {
	kind  string
	globs []string
}

func (rule fakeRule) Type() string { return rule.kind }

func (rule fakeRule) Globs() []string { return rule.globs }

var (
	_ Rule    = fakeRule{}
	_ Globber = fakeRule{}
)

var errFakeField = errors.New("fake: missing field")

func fakeConstructor(kind string) Constructor {
	return func(raw json.RawMessage) (Rule, error) {
		var body struct {
			Paths []string `json:"paths"`
		}

		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("decode fake rule: %w", err)
		}

		if len(body.Paths) == 0 {
			return nil, errFakeField
		}

		return fakeRule{kind: kind, globs: body.Paths}, nil
	}
}

func TestRegistryBuild(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(map[string]Constructor{
		"sql-fk": fakeConstructor("sql-fk"),
		"alpha":  fakeConstructor("alpha"),
	})

	testCases := []struct {
		name      string
		specs     string
		wantTypes []string
		wantErr   error
		wantText  string
	}{
		{"no rules", `[]`, []string{}, nil, ""},
		{
			"two known rules in order",
			`[{"type":"sql-fk","paths":["a/*.sql"]},{"type":"alpha","paths":["b"]}]`,
			[]string{"sql-fk", "alpha"},
			nil, "",
		},
		{
			"an unknown type lists the known ones sorted",
			`[{"type":"sql-fk","paths":["a"]},{"type":"nope"}]`,
			nil, ErrUnknownType, `rules[1]: unknown type "nope" (known: alpha, sql-fk)`,
		},
		{
			"a constructor error carries the index and type",
			`[{"type":"sql-fk","paths":["a"]},{"type":"alpha"}]`,
			nil, errFakeField, "rules[1] (alpha): fake: missing field",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			file, err := Decode([]byte(`{"rules":` + testCase.specs + `}`))
			if err != nil {
				t.Fatalf("decode specs: %v", err)
			}

			got, err := registry.Build(file.Rules)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Build() err = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("Build() err = %q, want it to contain %q", err, testCase.wantText)
			}

			if len(got) != len(testCase.wantTypes) {
				t.Fatalf("Build() = %d rules, want %d", len(got), len(testCase.wantTypes))
			}

			for idx, kind := range testCase.wantTypes {
				if got[idx].Type() != kind {
					t.Fatalf("rule %d = %s, want %s", idx, got[idx].Type(), kind)
				}
			}
		})
	}
}

func TestNewRegistryCopiesItsInput(t *testing.T) {
	t.Parallel()

	source := map[string]Constructor{"alpha": fakeConstructor("alpha")}
	registry := NewRegistry(source)
	delete(source, "alpha")

	_, err := registry.Build([]Spec{{Type: "alpha", Raw: json.RawMessage(`{"paths":["a"]}`)}})
	if err != nil {
		t.Fatalf("Build() = %v, want the registry to keep its own copy", err)
	}
}

func TestRegistryBuildWithoutTypes(t *testing.T) {
	t.Parallel()

	_, err := NewRegistry(nil).Build([]Spec{{Type: "go-di"}})

	want := `rules[0]: unknown type "go-di" (no rule types are registered in this build)`
	if !errors.Is(err, ErrUnknownType) || err.Error() != want {
		t.Fatalf("Build() err = %v, want %q wrapping ErrUnknownType", err, want)
	}
}
