package web

import (
	"slices"
	"testing"
)

func TestRouteRule(t *testing.T) {
	t.Parallel()

	type testCase struct {
		file string
		want []string
	}

	testCases := []testCase{
		{
			file: "web/src/routes/+page.svelte",
			want: []string{
				"web/src/routes/+page.server.dart:actions reference",
				"web/src/routes/+page.server.dart:load reference",
			},
		},
		{file: "web/src/routes/conversas/[id]/+page.svelte", want: nil},
		{file: "ui/svelte/atoms/Icon.svelte", want: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()

			if got := edgesFrom(t, routeRule{}, tc.file); !slices.Equal(got, tc.want) {
				t.Errorf("route edges = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRouteFor(t *testing.T) {
	t.Parallel()

	run := &index{scripts: []string{
		"web/src/routes/+page.svelte",
		"web/src/routes/conversas/+page.svelte",
		"web/src/routes/conversas/[id]/+page.svelte",
		"web/src/routes/conversas/novo/+page.svelte",
		"web/src/routes/conversas/+layout.svelte",
		"catalog/src/routes/funil/+page.svelte",
	}}

	type testCase struct {
		path    string
		want    string
		isFound bool
	}

	testCases := []testCase{
		{path: "/conversas", want: "web/src/routes/conversas/+page.svelte", isFound: true},
		{path: "/conversas/abc", want: "web/src/routes/conversas/[id]/+page.svelte", isFound: true},
		{
			path:    "/conversas/novo",
			want:    "web/src/routes/conversas/novo/+page.svelte",
			isFound: true,
		},
		{path: "/funil", isFound: false},
		{path: "/conversas/abc/midia", isFound: false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			got, found := run.routeFor("web", tc.path)
			if got != tc.want || found != tc.isFound {
				t.Errorf(
					"routeFor(%q) = %q, %v; want %q, %v",
					tc.path,
					got,
					found,
					tc.want,
					tc.isFound,
				)
			}
		})
	}
}
