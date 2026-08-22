package golang

import (
	"context"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSyntaxDeclarationsKindsAndNames(t *testing.T) {
	t.Parallel()

	const src = `package sample

import "fmt"

// Repo stores things.
type Repo struct{}

// Insert adds an item.
func (r *Repo) Insert() error { return nil }

type Other struct{}

// Insert on a different receiver must not collapse with Repo.Insert.
func (o Other) Insert() error { return nil }

// New builds a Repo.
func New() *Repo { return &Repo{} }

var Timeout = 5

const MaxRetries = 3

func unused() { fmt.Println() }
`

	syntax := NewSyntax()

	decls, err := syntax.Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	byName := make(map[string]entities.Decl, len(decls))
	for _, d := range decls {
		byName[d.Key.Name] = d
	}

	testCases := []struct {
		name     string
		declName string
		wantKind entities.DeclKind
	}{
		{"struct type", "Repo", entities.KindType},
		{"method keeps receiver", "Repo.Insert", entities.KindMethod},
		{"different receiver stays distinct", "Other.Insert", entities.KindMethod},
		{"constructor func", "New", entities.KindFunc},
		{"package-level var", "Timeout", entities.KindValue},
		{"package-level const", "MaxRetries", entities.KindValue},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decl, found := byName[testCase.declName]
			if !found {
				t.Fatalf("no decl named %q; have %v", testCase.declName, keys(byName))
			}

			if decl.Key.Kind != testCase.wantKind {
				t.Fatalf("Kind = %v, want %v", decl.Key.Kind, testCase.wantKind)
			}
		})
	}

	if _, collided := byName["Insert"]; collided {
		t.Fatalf("Repo.Insert and Other.Insert collapsed into a bare %q key", "Insert")
	}
}

func keys(m map[string]entities.Decl) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

func TestSyntaxDeclarationsSpanStartsAtDocComment(t *testing.T) {
	t.Parallel()

	const src = `package sample

// Greet says hello.
func Greet() {}
`

	syntax := NewSyntax()

	decls, err := syntax.Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	wantStart := len("package sample\n\n")

	if decls[0].Span.Start != wantStart {
		t.Fatalf(
			"Span.Start = %d, want %d (the doc comment, not the func keyword)",
			decls[0].Span.Start,
			wantStart,
		)
	}
}

func TestSyntaxDeclarationsSelectorIsTheName(t *testing.T) {
	t.Parallel()

	const src = "package sample\n\nfunc Greet() {}\n"

	syntax := NewSyntax()

	decls, err := syntax.Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	want := entities.Location{File: "sample.go", Line: 2, Char: len("func ")}
	if decls[0].Selector != want {
		t.Fatalf("Selector = %+v, want %+v", decls[0].Selector, want)
	}
}

func TestSyntaxDeclarationsIsTest(t *testing.T) {
	t.Parallel()

	const src = `package sample

import "testing"

func TestFoo(t *testing.T) {}
func Testify()              {}
func BenchmarkFoo(b *testing.B) {}
func helper()               {}
`

	syntax := NewSyntax()

	decls, err := syntax.Declarations(context.Background(), "sample_test.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	got := map[string]bool{}
	for _, d := range decls {
		got[d.Key.Name] = d.IsTest
	}

	testCases := []struct {
		name     string
		declName string
		want     bool
	}{
		{"TestFoo is a test", "TestFoo", true},
		{"Testify is not (lowercase after prefix)", "Testify", false},
		{"BenchmarkFoo is a test", "BenchmarkFoo", true},
		{"helper is not a test", "helper", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got[testCase.declName] != testCase.want {
				t.Fatalf(
					"IsTest[%s] = %v, want %v",
					testCase.declName,
					got[testCase.declName],
					testCase.want,
				)
			}
		})
	}
}

func TestSyntaxDeclarationsUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		src  string
		want entities.UnitID
	}{
		{
			"internal package",
			"pkg/repo.go",
			"package repo\n\nfunc F() {}\n",
			entities.UnitID("pkg"),
		},
		{
			"external test package is a different unit",
			"pkg/repo_test.go",
			"package repo_test\n\nfunc F() {}\n",
			entities.UnitID("pkg#repo_test"),
		},
		{
			"internal test file shares the unit",
			"pkg/repo_test.go",
			"package repo\n\nfunc F() {}\n",
			entities.UnitID("pkg"),
		},
	}

	syntax := NewSyntax()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := syntax.Declarations(
				context.Background(),
				testCase.path,
				[]byte(testCase.src),
			)
			if err != nil {
				t.Fatalf("Declarations() error = %v", err)
			}

			if decls[0].Unit != testCase.want {
				t.Fatalf("Unit = %q, want %q", decls[0].Unit, testCase.want)
			}
		})
	}
}

func TestSyntaxDeclarationsBlankIdentifierDoesNotCollapse(t *testing.T) {
	t.Parallel()

	const src = `package sample

import "io"

type ReaderA struct{}
type ReaderB struct{}

var _ io.Reader = (*ReaderA)(nil)
var _ io.Reader = (*ReaderB)(nil)
`

	syntax := NewSyntax()

	decls, err := syntax.Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	names := map[string]int{}

	for _, d := range decls {
		if d.Key.Kind == entities.KindValue {
			names[d.Key.Name]++
		}
	}

	if len(names) != 2 {
		t.Fatalf("want 2 distinct blank-identifier declarations, got %v", names)
	}

	for name, count := range names {
		if count != 1 {
			t.Fatalf("declaration %q appeared %d times", name, count)
		}
	}
}

func TestSyntaxDeclarationsNamesEveryIotaConstant(t *testing.T) {
	t.Parallel()

	const src = `package sample

type Weekday int

const (
	Sunday Weekday = iota
	Monday
	Tuesday
)
`

	decls, err := NewSyntax().Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	var values []entities.Decl

	for _, decl := range decls {
		if decl.Key.Kind == entities.KindValue {
			values = append(values, decl)
		}
	}

	// Naming the group after its first constant left Monday and Tuesday with
	// no key at all: nothing ever asked who referenced them, no birth was
	// recorded, and a consumer looking one up fell back to the file's birth -
	// which dates a constant that appears late to the day its file was made.
	if len(values) != 3 {
		t.Fatalf("got %d value declarations, want one per constant: %v", len(values), values)
	}

	names := map[string]entities.Span{}
	for _, decl := range values {
		names[decl.Key.Name] = decl.Span
	}

	for _, want := range []string{"Sunday", "Monday", "Tuesday"} {
		if _, found := names[want]; !found {
			t.Fatalf("%s has no declaration of its own", want)
		}
	}

	// They still share one span, which is what keeps the block whole when it
	// is rendered: an unwritten iota constant takes its value from where it
	// sits, so the block cannot be cut.
	if names["Sunday"] != names["Tuesday"] {
		t.Fatalf("spans differ (%v vs %v); the group would be splittable",
			names["Sunday"], names["Tuesday"])
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

func TestSyntaxDeclarationsUngroupedConstsSplit(t *testing.T) {
	t.Parallel()

	const src = `package sample

const (
	DefaultTimeout = 5
	MaxRetries     = 3
)
`

	syntax := NewSyntax()

	decls, err := syntax.Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	if len(decls) != 2 {
		t.Fatalf("independent const specs should split, got %d decls", len(decls))
	}
}

func TestSyntaxDeclarationsParseError(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax()

	_, err := syntax.Declarations(
		context.Background(),
		"broken.go",
		[]byte("package sample\nfunc ("),
	)
	if err == nil {
		t.Fatal("Declarations() error = nil, want a parse error")
	}
}

func TestReceiverName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
		want string
	}{
		{"pointer receiver", "package s\nfunc (r *Repo) M() {}\n", "Repo"},
		{"value receiver", "package s\nfunc (r Repo) M() {}\n", "Repo"},
		{"generic receiver", "package s\nfunc (r *Repo[T]) M() {}\n", "Repo"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			syntax := NewSyntax()

			decls, err := syntax.Declarations(context.Background(), "s.go", []byte(testCase.src))
			if err != nil {
				t.Fatalf("Declarations() error = %v", err)
			}

			want := testCase.want + ".M"
			if decls[0].Key.Name != want {
				t.Fatalf("method name = %q, want %q", decls[0].Key.Name, want)
			}
		})
	}
}
