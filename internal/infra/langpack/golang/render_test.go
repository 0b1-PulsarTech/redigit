package golang

import (
	"context"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// TestRenderCorpusRoundTrip is the renderer's own validation: rendering with
// every declaration kept must reproduce the gofmt-normalized source byte for
// byte. The corpus covers the hazards a naive go/ast.FilterFile hits: a
// build constraint, a //go:generate directive, blank-identifier assertions,
// a grouped const block, generics, and an external test package.
func TestRenderCorpusRoundTrip(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir("testdata/render")
	if err != nil {
		t.Fatalf("read testdata/render: %v", err)
	}

	syntax := NewSyntax()

	for _, entry := range entries {
		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("testdata/render", entry.Name())

			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			decls, err := syntax.Declarations(context.Background(), entry.Name(), source)
			if err != nil {
				t.Fatalf("Declarations() error = %v", err)
			}

			keep := make([]entities.DeclKey, len(decls))
			for i, d := range decls {
				keep[i] = d.Key
			}

			got, err := syntax.Render(context.Background(), source, keep)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			want, err := format.Source(source)
			if err != nil {
				t.Fatalf("format.Source(%s): %v", path, err)
			}

			if string(got) != string(want) {
				t.Fatalf(
					"Render with every decl kept != gofmt(source).\ngot:\n%s\nwant:\n%s",
					got,
					want,
				)
			}
		})
	}
}

func TestRenderEveryDeclDroppedIsEmpty(t *testing.T) {
	t.Parallel()

	const src = `package sample

import "fmt"

func Foo() { fmt.Println("x") }
`

	syntax := NewSyntax()

	got, err := syntax.Render(context.Background(), []byte(src), nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("Render with nothing kept = %q, want empty (not a bare package clause)", got)
	}
}

func TestRenderKeepsOnlyRequested(t *testing.T) {
	t.Parallel()

	const src = `package sample

import "fmt"

func Foo() { fmt.Println("foo") }

func Bar() { fmt.Println("bar") }
`

	syntax := NewSyntax()

	keep := []entities.DeclKey{{Kind: entities.KindFunc, Name: "Bar"}}

	got, err := syntax.Render(context.Background(), []byte(src), keep)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if strings.Contains(string(got), "Foo") {
		t.Fatalf("Render output still mentions the dropped decl:\n%s", got)
	}

	if !strings.Contains(string(got), "func Bar()") {
		t.Fatalf("Render output is missing the kept decl:\n%s", got)
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

	syntax := NewSyntax()

	keep := []entities.DeclKey{{Kind: entities.KindFunc, Name: "UseJSON"}}

	got, err := syntax.Render(context.Background(), []byte(src), keep)
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

	syntax := NewSyntax()

	keep := []entities.DeclKey{{Kind: entities.KindFunc, Name: "Foo"}}

	got, err := syntax.Render(context.Background(), []byte(src), keep)
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

func TestRenderSplitsGroupedConsts(t *testing.T) {
	t.Parallel()

	const src = `package sample

const (
	DefaultTimeout = 5
	MaxRetries     = 3
)
`

	syntax := NewSyntax()

	keep := []entities.DeclKey{{Kind: entities.KindValue, Name: "DefaultTimeout"}}

	got, err := syntax.Render(context.Background(), []byte(src), keep)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if strings.Contains(string(got), "MaxRetries") {
		t.Fatalf("Render kept a dropped sibling from a const group:\n%s", got)
	}

	if !strings.Contains(string(got), "DefaultTimeout") {
		t.Fatalf("Render dropped the kept spec:\n%s", got)
	}

	formatted, err := format.Source(got)
	if err != nil {
		t.Fatalf("Render output is not gofmt-clean: %v\n%s", err, got)
	}

	if string(formatted) != string(got) {
		t.Fatalf("Render output is not gofmt-clean.\ngot:\n%s\nformatted:\n%s", got, formatted)
	}
}

func TestRenderKeepsIotaGroupAtomic(t *testing.T) {
	t.Parallel()

	const src = `package sample

type Weekday int

const (
	Sunday Weekday = iota
	Monday
	Tuesday
)
`

	syntax := NewSyntax()

	testCases := []struct {
		name string
		keep []entities.DeclKey
		want string
	}{
		{
			"keeping the group's name keeps every member",
			[]entities.DeclKey{
				{Kind: entities.KindType, Name: "Weekday"},
				{Kind: entities.KindValue, Name: "Sunday"},
			},
			"Tuesday",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := syntax.Render(context.Background(), []byte(src), testCase.keep)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			if !strings.Contains(string(got), testCase.want) {
				t.Fatalf("iota group split apart; missing %q:\n%s", testCase.want, got)
			}
		})
	}
}

func TestRenderParseError(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax()

	_, err := syntax.Render(context.Background(), []byte("package sample\nfunc ("), nil)
	if err == nil {
		t.Fatal("Render() error = nil, want a parse error")
	}
}

// TestRenderKeepsAVersionedImport pins the defect that survived a 283-file
// round trip and only surfaced when a real repository was rendered: a module
// path carrying a major version is reached by the element BEFORE it, so taking
// the last segment yields "v2" or "v5", matches nothing the code writes, and
// the import is pruned. Every affected file then fails to compile.
//
// This repository's own dependencies happen to carry no version suffix, which
// is exactly why its corpus test passed while the tool produced six broken
// files out of 2484.
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

			decls, err := NewSyntax().Declarations(context.Background(), "a.go", source)
			if err != nil {
				t.Fatalf("Declarations() = %v", err)
			}

			keys := make([]entities.DeclKey, 0, len(decls))
			for _, decl := range decls {
				keys = append(keys, decl.Key)
			}

			out, err := NewSyntax().Render(context.Background(), source, keys)
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

	got, err := NewSyntax().Render(
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
