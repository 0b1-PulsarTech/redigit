package slots

import (
	"slices"
	"testing"
)

func TestParseGoMod(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		want    goMod
		wantOK  bool
	}{
		{
			"module with requires",
			"module ex/a\n\ngo 1.26.0\n\nrequire (\n\tex/b v1.0.0\n\tx/y v1.2.0 // indirect\n)\nrequire ex/c v0.0.0\n",
			goMod{path: "ex/a", version: "1.26.0", requires: []string{"ex/b", "x/y", "ex/c"}},
			true,
		},
		{"module only", "module ex/a\n", goMod{path: "ex/a"}, true},
		{
			"local replace targets only",
			"module ex/a\n\nreplace (\n\tex/b => ../b\n\tex/c => ./c\n\tex/d => ex/e v1.0.0\n)\n",
			goMod{path: "ex/a", replaces: []string{"../b", "./c"}},
			true,
		},
		{
			"a strict parse failure is opaque",
			"module ex/a\n\nfuture-directive x\n",
			goMod{path: "ex/a", isOpaque: true},
			true,
		},
		{"garbage", "this is not a go.mod\n", goMod{}, false},
		{"no module directive", "go 1.26\n", goMod{}, false},
		{"empty", "", goMod{}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, isOK := parseGoMod([]byte(testCase.content))
			if isOK != testCase.wantOK || got.path != testCase.want.path ||
				got.version != testCase.want.version || !slices.Equal(got.requires, testCase.want.requires) ||
				!slices.Equal(
					got.replaces,
					testCase.want.replaces,
				) || got.isOpaque != testCase.want.isOpaque {
				t.Fatalf(
					"got %+v ok = %v, want %+v ok = %v",
					got,
					isOK,
					testCase.want,
					testCase.wantOK,
				)
			}
		})
	}
}
