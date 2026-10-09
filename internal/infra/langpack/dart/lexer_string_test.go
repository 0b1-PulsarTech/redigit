package dart

import (
	"slices"
	"testing"
)

func TestLexStringLiteral(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name      string
		source    string
		wantValue string
		wantInner []string
	}

	testCases := []testCase{
		{"plain literal carries its value", `'plain'`, "plain", nil},
		{"escaped quote stays inside", `'it\'s'`, `it\'s`, nil},
		{"short interpolation lexes its name", `"a $b c"`, "", []string{"b"}},
		{
			"long interpolation lexes its expression",
			`'${c.d(e)}'`,
			"",
			[]string{"c", ".", "d", "(", "e", ")"},
		},
		{"braces inside an interpolation nest", `'${ {f} }'`, "", []string{"{", "f", "}"}},
		{"a brace in plain text is text", `'{'`, "{", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			toks := lex(tc.source)
			if len(toks) != 1 || toks[0].kind != tokString {
				t.Fatalf("lex() = %q, want one string token", texts(toks))
			}

			if toks[0].value != tc.wantValue {
				t.Fatalf("value = %q, want %q", toks[0].value, tc.wantValue)
			}

			if got := texts(
				toks[0].inner,
			); !slices.Equal(got, tc.wantInner) &&
				len(got)+len(tc.wantInner) > 0 {
				t.Fatalf("inner = %q, want %q", got, tc.wantInner)
			}
		})
	}
}
