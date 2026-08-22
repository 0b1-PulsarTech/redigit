package golang

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestGroupByFile(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		{Key: entities.DeclKey{File: "a.go", Name: "A"}},
		{Key: entities.DeclKey{File: "b.go", Name: "B"}},
		{Key: entities.DeclKey{File: "a.go", Name: "C"}},
	}

	got := groupByFile(decls)

	if len(got["a.go"]) != 2 {
		t.Fatalf("groupByFile()[a.go] has %d entries, want 2", len(got["a.go"]))
	}

	if len(got["b.go"]) != 1 {
		t.Fatalf("groupByFile()[b.go] has %d entries, want 1", len(got["b.go"]))
	}
}

func TestFindDecl(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		{Key: entities.DeclKey{File: "a.go", Kind: entities.KindType, Name: "Foo"}},
		{Key: entities.DeclKey{File: "b.go", Kind: entities.KindType, Name: "Foo"}},
	}

	testCases := []struct {
		name string
		file string
		kind entities.DeclKind
		key  string
		want bool
	}{
		{"exact match", "a.go", entities.KindType, "Foo", true},
		{"wrong file", "c.go", entities.KindType, "Foo", false},
		{"wrong kind", "a.go", entities.KindFunc, "Foo", false},
		{"wrong name", "a.go", entities.KindType, "Bar", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, found := findDecl(decls, testCase.file, testCase.kind, testCase.key)
			if found != testCase.want {
				t.Fatalf("findDecl() found = %v, want %v", found, testCase.want)
			}
		})
	}
}

func TestFindTypeNamed(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		{Key: entities.DeclKey{File: "a.go", Kind: entities.KindType, Name: "Foo"}},
		{Key: entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Bar"}},
	}

	testCases := []struct {
		name string
		key  string
		want bool
	}{
		{"type found", "Foo", true},
		{"kind mismatch (func, not type)", "Bar", false},
		{"no match", "Baz", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, found := findTypeNamed(decls, testCase.key)
			if found != testCase.want {
				t.Fatalf("findTypeNamed() found = %v, want %v", found, testCase.want)
			}
		})
	}
}
