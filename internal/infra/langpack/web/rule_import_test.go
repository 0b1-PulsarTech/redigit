package web

import (
	"slices"
	"testing"
)

func TestImportRule(t *testing.T) {
	t.Parallel()

	type testCase struct {
		file string
		want []string
	}

	testCases := []testCase{
		{
			file: "web/src/routes/+page.svelte",
			want: []string{
				"ui/svelte/atoms/Icon.svelte:<file> reference",
				"ui/svelte/format.ts:<file> reference",
				"web/src/lib/state.svelte.ts:<file> reference",
				"web/src/lib/util.ts:<file> reference",
				"web/src/virtual-app.d.ts:<file> reference",
				"web/tool/client_entry.dart:<file> reference",
			},
		},
		{file: "web/src/lib/util.ts", want: []string{"web/src/lib/more.ts:<file> reference"}},
		{file: "web/src/lib/b.ts", want: []string{"web/src/lib/a.ts:<file> reference"}},
		{file: "web/test/bare.mjs", want: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()

			if got := edgesFrom(t, importRule{}, tc.file); !slices.Equal(got, tc.want) {
				t.Errorf("import edges = %q, want %q", got, tc.want)
			}
		})
	}
}
