package web

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
)

func TestTarget(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		raw  string
		want string
	}

	testCases := []testCase{
		{name: "a path", raw: `"./svelte/*"`, want: "./svelte/*"},
		{
			name: "svelte wins over import",
			raw:  `{"import": "./a.js", "svelte": "./a.svelte"}`,
			want: "./a.svelte",
		},
		{
			name: "import before default",
			raw:  `{"default": "./d.js", "import": "./i.js"}`,
			want: "./i.js",
		},
		{name: "types as the last resort", raw: `{"types": "./t.d.ts"}`, want: "./t.d.ts"},
		{name: "nested conditions", raw: `{"import": {"types": "./t.d.ts"}}`, want: "./t.d.ts"},
		{name: "no known condition", raw: `{"node": "./n.js"}`, want: ""},
		{name: "neither a path nor conditions", raw: `["./a.js"]`, want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := target(json.RawMessage(tc.raw)); got != tc.want {
				t.Errorf("target() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExportsMap(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		exports string
		want    []string
	}

	testCases := []testCase{
		{
			name:    "subpaths",
			exports: `{"./*": "./s/*", "./format": "./f.ts"}`,
			want:    []string{"./*", "./format"},
		},
		{name: "a lone path is the root entry", exports: `"./index.js"`, want: []string{"."}},
		{
			name:    "conditions are the root entry",
			exports: `{"import": "./i.js"}`,
			want:    []string{"."},
		},
		{name: "no exports", exports: "", want: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := manifest{Exports: json.RawMessage(tc.exports)}.exportsMap()
			if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, tc.want) {
				t.Errorf("exportsMap() keys = %q, want %q", keys, tc.want)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()

	patterns := map[string][]string{
		"#lib/*":         {"./src/lib/*"},
		"#lib/server/*":  {"./server/*"},
		"virtual:client": {"./client.d.ts"},
		"#a/*.json":      {"./data/*.json"},
		"#empty":         {},
	}

	type testCase struct {
		name    string
		key     string
		want    string
		isFound bool
	}

	testCases := []testCase{
		{name: "an exact key", key: "virtual:client", want: "./client.d.ts", isFound: true},
		{name: "a wildcard", key: "#lib/util", want: "./src/lib/util", isFound: true},
		{
			name:    "the longest prefix wins",
			key:     "#lib/server/types",
			want:    "./server/types",
			isFound: true,
		},
		{name: "a wildcard with a suffix", key: "#a/x.json", want: "./data/x.json", isFound: true},
		{name: "a suffix that does not fit", key: "#a/x.ts", isFound: false},
		{name: "an exact key with no value", key: "#empty", want: "", isFound: true},
		{name: "no pattern", key: "svelte", isFound: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, found := match(patterns, tc.key, fillPaths)
			if got != tc.want || found != tc.isFound {
				t.Errorf("match(%q) = %q, %v; want %q, %v", tc.key, got, found, tc.want, tc.isFound)
			}
		})
	}
}

func TestNearest(t *testing.T) {
	t.Parallel()

	index := map[string]int{".": 0, "web": 1, "web/src/lib": 2}

	type testCase struct {
		name    string
		dir     string
		wantDir string
		isFound bool
	}

	testCases := []testCase{
		{name: "the directory itself", dir: "web/src/lib", wantDir: "web/src/lib", isFound: true},
		{name: "a parent", dir: "web/src/routes/x", wantDir: "web", isFound: true},
		{name: "the root", dir: "packages/ui", wantDir: ".", isFound: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, found := nearest(index, tc.dir)
			if got != tc.wantDir || found != tc.isFound {
				t.Errorf(
					"nearest(%q) = %q, %v; want %q, %v",
					tc.dir,
					got,
					found,
					tc.wantDir,
					tc.isFound,
				)
			}
		})
	}

	if dir, _, found := nearest(map[string]int{"web": 1}, "packages/ui"); found {
		t.Errorf("nearest() without a root = %q, want not found", dir)
	}
}
