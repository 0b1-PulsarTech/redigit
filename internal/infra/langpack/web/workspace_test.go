package web

import (
	"context"
	"errors"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	testCases := []resolveCase{
		{
			name: "relative without extension",
			from: "web/src/lib/a.ts",
			spec: "./b",
			want: "web/src/lib/b.ts",
		},
		{
			name: "relative .js written for .ts",
			from: "web/src/lib/util.ts",
			spec: "./more.js",
			want: "web/src/lib/more.ts",
		},
		{name: "a missing relative file", from: "web/src/lib/a.ts", spec: "./gone", want: ""},
		{
			name: "a # import",
			from: "web/src/routes/+page.svelte",
			spec: "#lib/util",
			want: "web/src/lib/util.ts",
		},
		{
			name: "a # import of a .svelte.ts",
			from: "web/src/routes/+page.svelte",
			spec: "#lib/state.svelte",
			want: "web/src/lib/state.svelte.ts",
		},
		{
			name: "an unmapped # import",
			from: "web/src/routes/+page.svelte",
			spec: "#tool/x",
			want: "",
		},
	}

	runResolveCases(t, app.space(), testCases)
}

func TestResolveAcrossPackages(t *testing.T) {
	t.Parallel()

	testCases := []resolveCase{
		{
			name: "a tsconfig path",
			from: "web/src/routes/+page.svelte",
			spec: "virtual:app",
			want: "web/src/virtual-app.d.ts",
		},
		{
			name: "a workspace package export",
			from: "web/src/routes/+page.svelte",
			spec: "ui/format",
			want: "ui/svelte/format.ts",
		},
		{
			name: "a wildcard export",
			from: "web/src/routes/+page.svelte",
			spec: "ui/atoms/Icon.svelte",
			want: "ui/svelte/atoms/Icon.svelte",
		},
		{
			name: "a package without the subpath",
			from: "web/src/routes/+page.svelte",
			spec: "ui",
			want: "",
		},
		{name: "a dependency", from: "web/src/routes/+page.svelte", spec: "svelte/store", want: ""},
		{
			name: "a scoped dependency",
			from: "web/src/routes/+page.svelte",
			spec: "@sveltejs/kit/node",
			want: "",
		},
		{name: "a Node built-in", from: "web/test/bare.mjs", spec: "node:fs", want: ""},
		{
			name: "a SvelteKit alias",
			from: "web/src/routes/+page.svelte",
			spec: "$app/forms",
			want: "",
		},
	}

	runResolveCases(t, app.space(), testCases)
}

type resolveCase struct {
	name string
	from string
	spec string
	want string
}

func runResolveCases(t *testing.T, space *workspace, testCases []resolveCase) {
	t.Helper()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := space.resolve(context.Background(), tc.from, tc.spec)
			if err != nil || got != tc.want {
				t.Errorf("resolve(%q) = %q, %v; want %q", tc.spec, got, err, tc.want)
			}
		})
	}
}

func TestResolveWorkspaceShapes(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		tree fakeTree
		spec string
		want string
	}

	testCases := []testCase{
		{
			name: "a scoped package without exports",
			tree: fakeTree{"p/package.json": `{"name": "@o/p"}`, "p/lib/x.ts": "", "a.ts": ""},
			spec: "@o/p/lib/x",
			want: "p/lib/x.ts",
		},
		{
			name: "a package's directory index",
			tree: fakeTree{"p/package.json": `{"name": "p"}`, "p/index.js": "", "a.ts": ""},
			spec: "p",
			want: "p/index.js",
		},
		{
			name: "a manifest that is not JSON resolves nothing",
			tree: fakeTree{"p/package.json": `{name: p}`, "p/index.js": "", "a.ts": ""},
			spec: "p",
			want: "",
		},
		{
			name: "a tsconfig with comments falls back to the package",
			tree: fakeTree{
				"tsconfig.json":  "// x\n{}",
				"p/package.json": `{"name": "p"}`,
				"p/index.ts":     "",
				"a.ts":           "",
			},
			spec: "p",
			want: "p/index.ts",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.tree.space().resolve(context.Background(), "a.ts", tc.spec)
			if err != nil || got != tc.want {
				t.Errorf("resolve(%q) = %q, %v; want %q", tc.spec, got, err, tc.want)
			}
		})
	}
}

func TestResolveUnreadableManifest(t *testing.T) {
	t.Parallel()

	space := newWorkspace([]string{"package.json", "a.ts"}, fakeTree{"a.ts": ""}.Read)

	if _, err := space.resolve(context.Background(), "a.ts", "./b"); !errors.Is(err, errMissing) {
		t.Errorf("resolve() error = %v, want %v", err, errMissing)
	}
}
