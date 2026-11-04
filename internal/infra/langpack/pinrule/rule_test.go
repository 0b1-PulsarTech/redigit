package pinrule

import (
	"slices"
	"testing"
)

func TestRuleGlobs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		raw       string
		wantGlobs []string
	}{
		{
			name:      "a glob on the decl side",
			raw:       `{"type":"pin","decl":"a/*_test.go:func:Test*","after":"b.go:func:B"}`,
			wantGlobs: []string{"a/*_test.go", "b.go"},
		},
		{
			name:      "plain keys",
			raw:       `{"type":"pin","decl":"a.go:func:A","after":"b.go:func:B"}`,
			wantGlobs: []string{"a.go", "b.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := Constructor("pin")([]byte(testCase.raw))
			if err != nil {
				t.Fatalf("Constructor() = %v", err)
			}

			if got := built.Type(); got != "pin" {
				t.Fatalf("Type() = %q, want pin", got)
			}

			globber, isGlobber := built.(interface{ Globs() []string })
			if !isGlobber {
				t.Fatalf("%T is no Globber", built)
			}

			if !slices.Equal(globber.Globs(), testCase.wantGlobs) {
				t.Fatalf("Globs() = %v, want %v", globber.Globs(), testCase.wantGlobs)
			}
		})
	}
}

func TestConstructorRefuses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		raw  string
	}{
		{name: "a pin without targets", raw: `{"type":"pin"}`},
		{name: "not an object", raw: `[1]`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Constructor("pin")([]byte(testCase.raw)); err == nil {
				t.Fatalf("Constructor(%s) accepted it", testCase.raw)
			}
		})
	}
}
