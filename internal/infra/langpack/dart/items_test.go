package dart

import (
	"slices"
	"testing"
)

func TestItems(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		source string
		want   []string
	}

	testCases := []testCase{
		{
			"a body closes an item, a semicolon after an initializer closes one",
			"class A { void f() {} }\nfinal x = {1};\nvoid g() => 1;\nint h() { return 1; }\n",
			[]string{
				"class A { void f() {} }\n",
				"final x = {1};\n",
				"void g() => 1;\n",
				"int h() { return 1; }\n",
			},
		},
		{
			"the first item takes only the comments touching it",
			"// header\n\n/// doc\n@meta\nclass A {}\n",
			[]string{"/// doc\n@meta\nclass A {}\n"},
		},
		{
			"a trailing comment stays on its item's line",
			"void g() => 1; // tail\nint y = 2;\n",
			[]string{"void g() => 1; // tail\n", "int y = 2;\n"},
		},
		{
			"two items on one line split where the first ends",
			"final x = {1}; int y = 2;\n",
			[]string{"final x = {1};", " int y = 2;\n"},
		},
		{
			"a string's braces never close an item",
			"final s = '}'; final t = '${ {1} }';\n",
			[]string{"final s = '}';", " final t = '${ {1} }';\n"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			toks := lex(tc.source)

			var got []string
			for _, it := range placeItems(tc.source, toks, splitItems(tc.source, toks)) {
				got = append(got, tc.source[it.start:it.end])
			}

			if !slices.Equal(got, tc.want) {
				t.Fatalf("items = %q, want %q", got, tc.want)
			}
		})
	}
}
