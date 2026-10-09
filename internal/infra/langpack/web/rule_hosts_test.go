package web

import (
	"slices"
	"testing"
)

func TestHostsRule(t *testing.T) {
	t.Parallel()

	type testCase struct {
		file string
		want []string
	}

	testCases := []testCase{
		{
			file: "web/src/routes/+page.svelte",
			want: []string{"web/package.json:<file> hosts", "web/tsconfig.json:<file> hosts"},
		},
		{file: "ui/svelte/format.ts", want: []string{"ui/package.json:<file> hosts"}},
		{file: "web/package.json", want: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()

			if got := edgesFrom(t, hostsRule{}, tc.file); !slices.Equal(got, tc.want) {
				t.Errorf("hosts edges = %q, want %q", got, tc.want)
			}
		})
	}
}
