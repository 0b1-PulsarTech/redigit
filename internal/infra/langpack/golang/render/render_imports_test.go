package render

import (
	"go/ast"
	"strconv"
	"testing"
)

func TestImportIsUsed(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		path  string
		alias string
		used  []string
		want  bool
	}{
		{name: "v1 as v1", path: "example.com/fx/v1", used: []string{"v1"}, want: true},
		{name: "v2 as v2", path: "example.com/fx/v2", used: []string{"v2"}, want: true},
		{name: "rand/v2 as rand", path: "math/rand/v2", used: []string{"rand"}, want: true},
		{name: "v2 as neither", path: "example.com/fx/v2", used: []string{"other"}},
		{name: "yaml.v3 as yaml", path: "gopkg.in/yaml.v3", used: []string{"yaml"}, want: true},
		{name: "aliased v1", path: "ex.com/fx/v1", alias: "mnt", used: []string{"mnt"}, want: true},
		{name: "aliased v1 not as v1", path: "ex.com/fx/v1", alias: "mnt", used: []string{"v1"}},
		{name: "plain path", path: "example.com/fx", used: []string{"fx"}, want: true},
		{name: "plain path unused", path: "example.com/fx", used: []string{"v1"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			spec := &ast.ImportSpec{Path: &ast.BasicLit{Value: strconv.Quote(testCase.path)}}
			if testCase.alias != "" {
				spec.Name = ast.NewIdent(testCase.alias)
			}

			used := map[string]bool{}
			for _, name := range testCase.used {
				used[name] = true
			}

			if got := importIsUsed(spec, used); got != testCase.want {
				t.Fatalf("got %v, want %v", got, testCase.want)
			}
		})
	}
}
