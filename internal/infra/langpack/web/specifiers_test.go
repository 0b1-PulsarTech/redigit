package web

import (
	"slices"
	"testing"
)

func TestSpecifiers(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		source string
		want   []string
	}

	testCases := []testCase{
		{
			name: "static, side-effect and dynamic imports in order",
			source: "import 'a.css';\nimport { b } from \"./b\";\n" +
				"const c = await import( './c.js' );\nexport * from '#lib/d';\n",
			want: []string{"a.css", "./b", "./c.js", "#lib/d"},
		},
		{
			name:   "a multi-line import list",
			source: "import {\n\tone,\n\ttwo\n} from 'x';\n",
			want:   []string{"x"},
		},
		{
			name:   "a type-only import",
			source: "import type { A } from './a';\n",
			want:   []string{"./a"},
		},
		{name: "no import", source: "<p>{from}</p>\n", want: []string{}},
		{name: "a specifier broken across lines", source: "from './a\n';\n", want: []string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := specifiers(tc.source); !slices.Equal(got, tc.want) {
				t.Errorf("specifiers() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVisits(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		source string
		want   []string
	}

	testCases := []testCase{
		{name: "a quoted path", source: "get('/conversas')", want: []string{"/conversas"}},
		{
			name:   "a nested path with a slash",
			source: `go("/funil/kanban/")`,
			want:   []string{"/funil/kanban"},
		},
		{name: "a query string", source: "fetch(`/funil?periodo=7`)", want: []string{"/funil"}},
		{name: "a fragment", source: "'/ajustes#ia'", want: []string{"/ajustes"}},
		{name: "the root path", source: "get('/')", want: nil},
		{name: "a file path", source: "read('/tmp/x.json')", want: nil},
		{name: "an upper-case segment", source: "'/Funil'", want: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := visits(tc.source); !slices.Equal(got, tc.want) {
				t.Errorf("visits() = %q, want %q", got, tc.want)
			}
		})
	}
}
