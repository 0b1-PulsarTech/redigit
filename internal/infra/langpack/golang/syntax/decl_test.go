package syntax

import (
	"context"
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

	decls, err := Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	byName := make(map[string]entities.Decl, len(decls))
	for _, decl := range decls {
		byName[decl.Key.Name] = decl
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

func keys(decls map[string]entities.Decl) []string {
	out := make([]string, 0, len(decls))
	for name := range decls {
		out = append(out, name)
	}

	return out
}

func TestSyntaxDeclarationsSpanStartsAtDocComment(t *testing.T) {
	t.Parallel()

	const src = `package sample

// Greet says hello.
func Greet() {}
`

	decls, err := Declarations(context.Background(), "sample.go", []byte(src))
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

	decls, err := Declarations(context.Background(), "sample.go", []byte(src))
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

	decls, err := Declarations(context.Background(), "sample_test.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	got := map[string]bool{}
	for _, decl := range decls {
		got[decl.Key.Name] = decl.IsTest
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

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := Declarations(
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

	decls, err := Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	names := map[string]int{}

	for _, decl := range decls {
		if decl.Key.Kind == entities.KindValue {
			names[decl.Key.Name]++
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

	decls, err := Declarations(context.Background(), "sample.go", []byte(src))
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

func TestSyntaxDeclarationsUngroupedConstsSplit(t *testing.T) {
	t.Parallel()

	const src = `package sample

const (
	DefaultTimeout = 5
	MaxRetries     = 3
)
`

	decls, err := Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	if len(decls) != 2 {
		t.Fatalf("independent const specs should split, got %d decls", len(decls))
	}
}

func TestSyntaxDeclarationsParseError(t *testing.T) {
	t.Parallel()

	_, err := Declarations(
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

			decls, err := Declarations(context.Background(), "s.go", []byte(testCase.src))
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

func TestSelectorCountsUTF16Units(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		source   string
		declName string
		wantLine int
		wantChar int
	}{
		{
			"an ascii line is the case where bytes and units agree",
			"package a\n\nfunc Add() {}\n",
			"Add",
			2,
			5,
		},
		{
			"a two-byte receiver name shifts the byte column but not the unit column",
			"package a\n\ntype Ação struct{}\n\nfunc (a Ação) Run() {}\n",
			"Ação.Run",
			4,
			14,
		},
		{
			"an emoji in a comment does not move a later line",
			"package a\n\n// \U0001F600 note\nfunc Add() {}\n",
			"Add",
			3,
			5,
		},
		{
			"a value declared after a two-byte name on the same line",
			"package a\n\nvar café, x = 1, 2\n",
			"x",
			2,
			10,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := Declarations(
				context.Background(),
				"a.go",
				[]byte(testCase.source),
			)
			if err != nil {
				t.Fatalf("Declarations() = %v", err)
			}

			var found entities.Decl

			for _, decl := range decls {
				if decl.Key.Name == testCase.declName {
					found = decl
				}
			}

			if found.Key.Name == "" {
				t.Fatalf("no declaration named %q among %d", testCase.declName, len(decls))
			}

			if found.Selector.Line != testCase.wantLine {
				t.Fatalf("selector line = %d, want %d", found.Selector.Line, testCase.wantLine)
			}

			if found.Selector.Char != testCase.wantChar {
				t.Fatalf("selector char = %d, want %d", found.Selector.Char, testCase.wantChar)
			}
		})
	}
}

func TestInterfaceIsMarked(t *testing.T) {
	t.Parallel()

	source := []byte(`package a

type Reader interface{ Read() error }

type Repo struct{}

func (Repo) Read() error { return nil }

type Alias = Reader

type Names []string
`)

	decls, err := Declarations(context.Background(), "a.go", source)
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	marked := map[string]bool{}
	for _, decl := range decls {
		marked[decl.Key.Name] = decl.IsInterface
	}

	testCases := []struct {
		name string
		decl string
		want bool
	}{
		{"an interface is marked", "Reader", true},
		{"a struct is not", "Repo", false},
		{"a named slice is not", "Names", false},
		{
			"an alias to an interface is not marked; the server answers about the alias",
			"Alias",
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := marked[testCase.decl]; got != testCase.want {
				t.Fatalf("%s IsInterface = %v, want %v", testCase.decl, got, testCase.want)
			}
		})
	}
}

func TestGroupSizeCountsTheBlockNotTheFile(t *testing.T) {
	t.Parallel()

	source := []byte(`package a

type Kind uint8

const (
	KindFixed Kind = iota
	KindPercentage
)

type Colour uint8

const (
	Red Colour = iota
	Green
	Blue
)

const Loose = 7

var (
	First  = 1
	Second = 2
)
`)

	decls, err := Declarations(context.Background(), "a.go", source)
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	size := map[string]int{}
	for _, decl := range decls {
		size[decl.Key.Name] = decl.GroupSize
	}

	testCases := []struct {
		name string
		decl string
		want int
	}{
		{"an enum of two is still a group", "KindFixed", 2},
		{"and its sibling agrees", "KindPercentage", 2},
		{"a second enum in the same file counts its own block", "Red", 3},
		{"a constant on its own is a group of one", "Loose", 1},
		{"a var block counts too", "First", 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := size[testCase.decl]; got != testCase.want {
				t.Fatalf("%s GroupSize = %d, want %d", testCase.decl, got, testCase.want)
			}
		})
	}
}

func TestDeclarationsMarkAGeneratedFileAsOneAtom(t *testing.T) {
	t.Parallel()

	source := []byte("// Code generated by MockGen. DO NOT EDIT.\n" +
		"package store\n\n" +
		"type MockReader struct{}\n\n" +
		"type MockReaderMockRecorder struct{}\n\n" +
		"func NewMockReader() *MockReader { return &MockReader{} }\n")

	decls, err := Declarations(t.Context(), "store/reader_mock_test.go", source)
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	if len(decls) != 3 {
		t.Fatalf("a generated file became %d declarations, want its 3", len(decls))
	}

	for _, decl := range decls {
		if decl.Atom != "store/reader_mock_test.go" {
			t.Fatalf("%s carries atom %q, want the file", decl.Key.Name, decl.Atom)
		}
	}
}

func TestDeclarationsStillCutAHandWrittenFile(t *testing.T) {
	t.Parallel()

	source := []byte(
		"package flow\n\ntype storeMock struct{}\n\nfunc (storeMock) Read() error { return nil }\n",
	)

	decls, err := Declarations(t.Context(), "flow/store_mock_test.go", source)
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	if len(decls) != 2 {
		t.Fatalf("a hand-written file became %d declarations, want 2", len(decls))
	}

	for _, decl := range decls {
		if decl.Atom != "" {
			t.Fatalf("%s carries atom %q: a person maintains this file", decl.Key.Name, decl.Atom)
		}
	}
}

func TestDeclarationsLeaveASqlcFileToTheSqlcRule(t *testing.T) {
	t.Parallel()

	source := []byte(
		"// Code generated by sqlc. DO NOT EDIT.\n\npackage dbgen\n\ntype Queries struct{}\n",
	)

	decls, err := Declarations(t.Context(), "dbgen/db.go", source)
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	for _, decl := range decls {
		if decl.Atom != "" {
			t.Fatalf("%s carries atom %q, want none", decl.Key.Name, decl.Atom)
		}
	}
}
