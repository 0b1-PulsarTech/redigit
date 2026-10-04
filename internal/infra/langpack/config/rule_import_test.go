package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// The first package importing a dependency was born hundreds of commits before
// the require that provides it: "no required module provides package".
func TestImportRuleMakesAFileWaitForTheRequireProvidingItsImports(t *testing.T) {
	t.Parallel()

	const (
		appMod = "apps/lead/go.mod"
		appGo  = "apps/lead/x.go"
	)

	requireOf := func(module string) entities.Decl {
		return piece(appMod, "require "+module, entities.Span{})
	}

	testCases := []struct {
		name    string
		source  string
		extra   []entities.Decl
		want    []string
		wantNot []string
	}{
		{
			name:   "an external package waits for its module",
			source: "package lead\n\nimport \"entgo.io/ent/dialect/sql\"\n",
			extra:  []entities.Decl{requireOf("entgo.io/ent")},
			want:   []string{"entgo.io/ent"},
		},
		{
			name:   "the longest required prefix owns the import",
			source: "package lead\n\nimport \"example.com/a/b/pkg\"\n",
			extra:  []entities.Decl{requireOf("example.com/a"), requireOf("example.com/a/b")},
			want:   []string{"example.com/a/b"},
			wantNot: []string{
				"example.com/a",
			},
		},
		{
			name:    "a prefix that is not a path boundary does not match",
			source:  "package lead\n\nimport \"example.com/ab/pkg\"\n",
			extra:   []entities.Decl{requireOf("example.com/a")},
			wantNot: []string{"example.com/a"},
		},
		{
			name:    "the standard library waits for nothing",
			source:  "package lead\n\nimport \"context\"\n",
			extra:   []entities.Decl{requireOf("example.com/a")},
			wantNot: []string{"example.com/a"},
		},
		{
			name:    "a file that does not parse yields no edge",
			source:  "this is not go",
			extra:   []entities.Decl{requireOf("example.com/a")},
			wantNot: []string{"example.com/a"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap := treeWith(t, map[string]string{appGo: testCase.source, appMod: "module m\n"})
			header := piece(appMod, "github.com/org/lead", entities.Span{})
			decl := goDecl(appGo, "Handler")
			decls := append([]entities.Decl{header, decl}, testCase.extra...)

			edges, err := (importRule{}).Edges(t.Context(), snap, decls)
			if err != nil {
				t.Fatalf("Edges() = %v", err)
			}

			assertRequireEdges(t, edges, decl.Key, testCase.want, testCase.wantNot)
		})
	}
}

func assertRequireEdges(
	t *testing.T,
	edges []entities.Edge,
	dependent entities.DeclKey,
	want, wantNot []string,
) {
	t.Helper()

	requireKey := func(module string) entities.DeclKey {
		return entities.DeclKey{
			File: "apps/lead/go.mod", Kind: entities.KindOther, Name: "require " + module,
		}
	}

	for _, module := range want {
		if !edgeBetween(edges, dependent, requireKey(module)) {
			t.Errorf("the file does not wait for require %s; edges: %v", module, edges)
		}
	}

	for _, module := range wantNot {
		if edgeBetween(edges, dependent, requireKey(module)) {
			t.Errorf("the file waits for require %s it does not import", module)
		}
	}
}

// A file belongs to the NEAREST go.mod: the outer module does not compile the
// inner one's packages, so its requires say nothing about them.
func TestImportRuleReadsTheNearestModulesRequires(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"go.mod":         "module outer\n",
		"inner/go.mod":   "module inner\n",
		"inner/pkg/x.go": "package pkg\n\nimport \"example.com/a\"\n",
	})

	outerHeader := piece("go.mod", "outer", entities.Span{})
	innerHeader := piece("inner/go.mod", "inner", entities.Span{})
	outerRequire := piece("go.mod", "require example.com/a", entities.Span{})
	innerRequire := piece("inner/go.mod", "require example.com/a", entities.Span{})
	decl := goDecl("inner/pkg/x.go", "F")

	edges, err := (importRule{}).Edges(
		t.Context(), snap,
		[]entities.Decl{outerHeader, innerHeader, outerRequire, innerRequire, decl},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, decl.Key, innerRequire.Key) {
		t.Error("the file does not wait for the inner module's require")
	}

	if edgeBetween(edges, decl.Key, outerRequire.Key) {
		t.Error("the file waits for the outer module's require")
	}
}
