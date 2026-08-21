package indexing

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestLocatorEnclosing(t *testing.T) {
	t.Parallel()

	source := []byte(
		"package a\n\nimport \"fmt\"\n\nfunc One() {\n\tfmt.Print()\n}\n\nfunc Two() {}\n",
	)
	one := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "One"}
	two := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Two"}

	where := newLocator()
	where.add("a.go", source, []entities.Decl{
		{Key: two, Span: entities.Span{Start: 55, End: 69}},
		{Key: one, Span: entities.Span{Start: 25, End: 54}},
	})

	testCases := []struct {
		name      string
		at        entities.Location
		wantKey   entities.DeclKey
		wantFound bool
	}{
		{
			"inside the first declaration",
			entities.Location{File: "a.go", Line: 5, Char: 2},
			one,
			true,
		},
		{"inside the second", entities.Location{File: "a.go", Line: 8, Char: 5}, two, true},
		{
			"the import block belongs to no declaration, which is an answer not a failure",
			entities.Location{File: "a.go", Line: 2, Char: 8},
			entities.DeclKey{},
			false,
		},
		{
			"an unknown file",
			entities.Location{File: "b.go", Line: 0, Char: 0},
			entities.DeclKey{},
			false,
		},
		{
			"a line past the file",
			entities.Location{File: "a.go", Line: 99, Char: 0},
			entities.DeclKey{},
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			key, found := where.Enclosing(testCase.at)
			if found != testCase.wantFound {
				t.Fatalf("Enclosing() found = %v, want %v", found, testCase.wantFound)
			}

			if key != testCase.wantKey {
				t.Fatalf("Enclosing() = %v, want %v", key, testCase.wantKey)
			}
		})
	}
}

// TestLocatorGapBetweenDeclarations pins that a location in the blank space
// after a declaration ends is not attributed to it. Reading one byte past a
// span as still inside it would invent an edge from whatever sits in the gap.
func TestLocatorGapBetweenDeclarations(t *testing.T) {
	t.Parallel()

	source := []byte("package a\n\nfunc A() {}\n\n// a loose comment\n\nfunc B() {}\n")
	where := newLocator()
	where.add("a.go", source, []entities.Decl{
		{
			Key:  entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "A"},
			Span: entities.Span{Start: 11, End: 22},
		},
	})

	if _, found := where.Enclosing(entities.Location{File: "a.go", Line: 4, Char: 3}); found {
		t.Fatal("a location in the gap after a declaration was attributed to it")
	}
}
