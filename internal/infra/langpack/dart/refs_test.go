package dart

import (
	"slices"
	"testing"
)

const useHeader = `import 'package:app/app.dart';
import 'src/money.dart';
import 'package:http/http.dart' as http;
import 'package:meta/meta.dart' show immutable;
import 'src/hidden.dart' as h;

class Own {}

`

// referencesOf scans the last declaration of a file holding useHeader and
// then body, naming each reference "provider@import".
func referencesOf(t *testing.T, body string) []string {
	t.Helper()

	found := scopeFor(t, useHeader+body)
	parsed := parseFile("lib/use.dart", useHeader+body)
	last := parsed.decls()[len(parsed.decls())-1]
	out := refNames(found.references(last.tokens(parsed.toks), last.key))

	slices.Sort(out)

	return out
}

func TestReferences(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		body string
		want []string
	}

	testCases := []testCase{
		{
			"a name two imports offer uses both",
			"Money m = Money();",
			[]string{"Money@0", "Money@0", "Money@1", "Money@1"},
		},
		{
			"the library's own name shadows every import",
			"Own o = Own();",
			[]string{"Own@-1", "Own@-1"},
		},
		{"a name a hide list removes resolves nowhere", "void f() { Secret; }", nil},
		{"a prefix reaches its own import only", "void f() { h.Secret(); }", []string{"Secret@4"}},
		{"an outside prefix counts as used", "void f() { http.get(); }", []string{"@2"}},
		{"an outside show list counts as used", "@immutable\nclass C {}", []string{"@3"}},
		{
			"a member after a dot reaches the extension",
			"void f(Money m) { m.label(); }",
			[]string{"Labels@0", "Labels@1", "Money@0", "Money@1"},
		},
		{
			"a member on an implicit receiver reaches the extension",
			"void f() { label(); }",
			[]string{"Labels@0", "Labels@1"},
		},
		{"a named argument's label is no reference", "void f() { g(Other: 1); }", nil},
		{"an interpolation is scanned", "void f() { '${Own}'; }", []string{"Own@-1"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := referencesOf(t, tc.body); !slices.Equal(got, tc.want) {
				t.Fatalf("references = %q, want %q", got, tc.want)
			}
		})
	}
}
