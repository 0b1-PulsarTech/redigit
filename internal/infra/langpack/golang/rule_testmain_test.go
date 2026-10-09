package golang

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestTestMainRuleAtoms(t *testing.T) {
	t.Parallel()

	test := func(file, name, unit string, start int) entities.Decl {
		return entities.Decl{
			Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
			Unit: entities.UnitID(unit),
			Span: entities.Span{Start: start},
		}
	}

	decls := []entities.Decl{
		test("a/main_test.go", "TestMain", "a", 0),
		test("a/z_test.go", "TestLast", "a", 0),
		test("a/b_test.go", "TestSecond", "a", 90),
		test("a/b_test.go", "TestFirst", "a", 10),
		test("c/c_test.go", "TestAlone", "c", 0),
		test("d/d_test.go", "TestMain", "d", 0),
		test("e/x_test.go", "TestMain", "e#e_test", 0),
		test("e/x_test.go", "TestExternal", "e#e_test", 20),
		test("e/e_test.go", "TestInternal", "e", 0),
	}

	atoms, err := testMainRule{}.Atoms(context.Background(), entities.Snapshot{}, decls)
	if err != nil {
		t.Fatalf("Atoms() error = %v", err)
	}

	testCases := []struct {
		name  string
		left  entities.Decl
		right entities.Decl
		same  bool
	}{
		{"main joins the first test by file then offset", decls[0], decls[3], true},
		{"main leaves later tests", decls[0], decls[2], false},
		{"main leaves a later file", decls[0], decls[1], false},
		{"external package keeps its own pair", decls[6], decls[7], true},
		{"external pair stays apart from the internal test", decls[6], decls[8], false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			left, right := atoms[testCase.left.Key], atoms[testCase.right.Key]
			if got := left != "" && left == right; got != testCase.same {
				t.Fatalf("atoms = %q and %q, same = %v, want %v", left, right, got, testCase.same)
			}
		})
	}

	if _, grouped := atoms[decls[5].Key]; grouped {
		t.Fatal("TestMain with no test in its package must group nothing")
	}
}
