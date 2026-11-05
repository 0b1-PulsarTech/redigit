package render

import (
	"context"
	"go/ast"
	"go/format"
	"strconv"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
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

func TestRenderPrunesUnreachedImports(t *testing.T) {
	t.Parallel()

	const src = `package sample

import (
	"encoding/json"
	"fmt"
)

func UseJSON() { _ = json.Marshal }

func UseFmt() { fmt.Println() }
`

	keep := []entities.DeclKey{{Kind: entities.KindFunc, Name: "UseJSON"}}

	got, err := Render(context.Background(), []byte(src), keep)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if strings.Contains(string(got), `"fmt"`) {
		t.Fatalf("Render kept an import nothing surviving reaches:\n%s", got)
	}

	if !strings.Contains(string(got), `"encoding/json"`) {
		t.Fatalf("Render dropped an import the surviving decl reaches:\n%s", got)
	}

	formatted, err := format.Source(got)
	if err != nil {
		t.Fatalf("Render output is not gofmt-clean: %v\n%s", err, got)
	}

	if string(formatted) != string(got) {
		t.Fatalf("Render output is not gofmt-clean.\ngot:\n%s\nformatted:\n%s", got, formatted)
	}
}

func TestRenderKeepsBlankAndDotImports(t *testing.T) {
	t.Parallel()

	const src = `package sample

import (
	_ "embed"

	. "strings"
)

func Foo() {}
`

	keep := []entities.DeclKey{{Kind: entities.KindFunc, Name: "Foo"}}

	got, err := Render(context.Background(), []byte(src), keep)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	testCases := []struct {
		name string
		want string
	}{
		{"blank import kept", `_ "embed"`},
		{"dot import kept", `. "strings"`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(string(got), testCase.want) {
				t.Fatalf("Render output missing %q:\n%s", testCase.want, got)
			}
		})
	}
}

func TestRenderKeepsAVersionedImport(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		want   string
	}{
		{
			"a standard library path with a major version",
			"package a\n\nimport \"math/rand/v2\"\n\nfunc Roll() int { return rand.N(6) }\n",
			"math/rand/v2",
		},
		{
			"a module path with a major version",
			"package a\n\nimport \"github.com/golang-jwt/jwt/v5\"\n\nfunc New() *jwt.Token { return nil }\n",
			"github.com/golang-jwt/jwt/v5",
		},
		{
			"a version riding on the element itself",
			"package a\n\nimport \"gopkg.in/yaml.v3\"\n\nfunc Parse(b []byte) error { return yaml.Unmarshal(b, nil) }\n",
			"gopkg.in/yaml.v3",
		},
		{
			"an alias still wins over any guess",
			"package a\n\nimport jwtlib \"github.com/golang-jwt/jwt/v5\"\n\nfunc New() *jwtlib.Token { return nil }\n",
			"github.com/golang-jwt/jwt/v5",
		},
		{
			"an element that merely starts with v is not a version",
			"package a\n\nimport \"example.com/pkg/vendor\"\n\nfunc Use() { vendor.Do() }\n",
			"example.com/pkg/vendor",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)

			decls, err := syntax.Declarations(context.Background(), "a.go", source)
			if err != nil {
				t.Fatalf("Declarations() = %v", err)
			}

			keys := make([]entities.DeclKey, 0, len(decls))
			for _, decl := range decls {
				keys = append(keys, decl.Key)
			}

			out, err := Render(context.Background(), source, keys)
			if err != nil {
				t.Fatalf("Render() = %v", err)
			}

			if !strings.Contains(string(out), testCase.want) {
				t.Fatalf("keeping every declaration dropped %q:\n%s", testCase.want, out)
			}

			// Keeping everything must reproduce the file exactly; an import
			// block that merely still holds the path but moved groups is also
			// a defect.
			if string(out) != testCase.source {
				t.Fatalf("round trip diverged:\nwant %q\ngot  %q", testCase.source, out)
			}
		})
	}
}

func TestRenderPrunesAnImportShadowedByALocal(t *testing.T) {
	t.Parallel()

	const src = `package sample

import "example.com/m/operator"

func Use(items []Item) {
	for _, operator := range items {
		_ = operator.ID
	}
}

var _ operator.Reader = Item{}
`

	got, err := Render(
		context.Background(),
		[]byte(src), []entities.DeclKey{{Kind: entities.KindFunc, Name: "Use"}},
	)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if strings.Contains(string(got), "example.com/m/operator") {
		t.Fatalf("Render kept an import only a local variable of that name reaches:\n%s", got)
	}
}

func TestRenderKeepsImportGroupsWhenSiblingsAreDropped(t *testing.T) {
	t.Parallel()

	const src = "package p\n\nimport (\n\t\"database/sql\"\n\t\"encoding/json\"\n\t\"time\"\n\n" +
		"\t\"example.com/ids\"\n\t\"example.com/pg\"\n)\n\n" +
		"type A struct {\n\tN sql.NullString\n\tT time.Time\n\tI ids.ID\n}\n\n" +
		"type B struct{ J json.RawMessage; P pg.Text }\n"

	got, err := Render(
		context.Background(), []byte(src),
		[]entities.DeclKey{{Kind: entities.KindType, Name: "A"}},
	)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "package p\n\nimport (\n\t\"database/sql\"\n\t\"time\"\n\n\t\"example.com/ids\"\n)\n\n" +
		"type A struct {\n\tN sql.NullString\n\tT time.Time\n\tI ids.ID\n}\n"
	if string(got) != want {
		t.Fatalf("Render() =\n%s\nwant\n%s", got, want)
	}
}
