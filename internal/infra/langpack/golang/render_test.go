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

func TestRenderSlicesSqlcOutputAndKeepsItsMarker(t *testing.T) {
	t.Parallel()

	const src = "// Code generated by sqlc. DO NOT EDIT.\n// versions:\n//   sqlc v1.31.1\n\n" +
		"package dbgen\n\nconst Get = `-- name: Get :one`\n\nfunc (q *Queries) Get() {}\n\n" +
		"const List = `-- name: List :many`\n\nfunc (q *Queries) List() {}\n"

	keep := []entities.DeclKey{
		{Kind: entities.KindValue, Name: "List"},
		{Kind: entities.KindMethod, Name: "Queries.List"},
	}

	got, err := NewSyntax().Render(context.Background(), []byte(src), keep)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "// Code generated by sqlc. DO NOT EDIT.\n// versions:\n//   sqlc v1.31.1\n\n" +
		"package dbgen\n\nconst List = `-- name: List :many`\n\nfunc (q *Queries) List() {}\n"
	if string(got) != want {
		t.Fatalf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderKeepsImportGroupsWhenSiblingsAreDropped(t *testing.T) {
	t.Parallel()

	const src = "package p\n\nimport (\n\t\"database/sql\"\n\t\"encoding/json\"\n\t\"time\"\n\n" +
		"\t\"example.com/ids\"\n\t\"example.com/pg\"\n)\n\n" +
		"type A struct {\n\tN sql.NullString\n\tT time.Time\n\tI ids.ID\n}\n\n" +
		"type B struct{ J json.RawMessage; P pg.Text }\n"

	got, err := NewSyntax().Render(
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

// TestRenderKeepsAnIotaGroupFromAnyMember pins the other half: naming any one
// constant of the block renders the whole block.
func TestRenderKeepsAnIotaGroupFromAnyMember(t *testing.T) {
	t.Parallel()

	const src = `package sample

const (
	Sunday int = iota
	Monday
	Tuesday
)
`

	testCases := []struct {
		name string
		keep string
	}{
		{"the first constant", "Sunday"},
		{"a middle one", "Monday"},
		{"the last", "Tuesday"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out, err := NewSyntax().Render(context.Background(), []byte(src), []entities.DeclKey{
				{File: "sample.go", Kind: entities.KindValue, Name: testCase.keep},
			})
			if err != nil {
				t.Fatalf("Render() = %v", err)
			}

			for _, want := range []string{"Sunday", "Monday", "Tuesday"} {
				if !strings.Contains(string(out), want) {
					t.Fatalf("keeping %s dropped %s:\n%s", testCase.keep, want, out)
				}
			}
		})
	}
}

// TestDeclarationlessFileIsCarried pins the case real source found and no
// fixture had: a file holding a package clause and imports and nothing else.
// With no declaration to stand for it, the file belongs to no commit, is never
// written, and vanishes from the rebuilt tree without the tree invariant ever
// naming it.
func TestDeclarationlessFileIsCarried(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
	}{
		{
			"a file that only pins blank imports",
			"package infra\n\nimport (\n\t_ \"example.com/a\"\n\t_ \"example.com/b\"\n)\n",
		},
		{
			"a package doc file with no imports at all",
			"// Package thing does a thing.\npackage thing\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)

			decls, err := NewSyntax().Declarations(context.Background(), "deps.go", source)
			if err != nil {
				t.Fatalf("Declarations() = %v", err)
			}

			if len(decls) != 1 {
				t.Fatalf("got %d declarations, want the one standing for the file", len(decls))
			}

			if decls[0].Key.Kind != entities.KindOther {
				t.Fatalf("kind = %q, want %q", decls[0].Key.Kind, entities.KindOther)
			}

			rendered, err := NewSyntax().Render(
				context.Background(),
				source,
				[]entities.DeclKey{decls[0].Key},
			)
			if err != nil {
				t.Fatalf("Render() = %v", err)
			}

			if string(rendered) != testCase.source {
				t.Fatalf("Render() = %q, want the file verbatim", rendered)
			}
		})
	}
}

// TestDeclarationlessFileRendersEmptyBeforeItIsBorn pins the other half: until
// its declaration is born, the file is not written at all, rather than being
// born holding only a package clause.
func TestDeclarationlessFileRendersEmptyBeforeItIsBorn(t *testing.T) {
	t.Parallel()

	source := []byte("package infra\n\nimport _ \"example.com/a\"\n")

	rendered, err := NewSyntax().Render(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}

	if len(rendered) != 0 {
		t.Fatalf("Render() = %q, want nothing", rendered)
	}
}

// Rendering a generated file must reproduce it byte for byte: the generator
// wrote every byte, and filtering through the printer drops the marker comment
// that says so.
func TestRenderReproducesAGeneratedFile(t *testing.T) {
	t.Parallel()

	source := []byte("// Code generated by MockGen. DO NOT EDIT.\n" +
		"package store\n\ntype MockReader struct{}\n")

	decls, err := NewSyntax().Declarations(t.Context(), "store/reader_mock_test.go", source)
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	keep := make([]entities.DeclKey, 0, len(decls))
	for _, decl := range decls {
		keep = append(keep, decl.Key)
	}

	got, err := NewSyntax().Render(t.Context(), source, keep)
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}

	if string(got) != string(source) {
		t.Fatalf("Render() =\n%q\nwant\n%q", got, source)
	}
}
