package syntax

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestReturnedElementsCutsOnlyWhatIsSyntacticallyAList(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		result string
		want   int
	}{
		{"a slice literal is cut by element", "[]step{a, b, c}", 3},
		{"an array literal is cut by element", "[2]step{a, b}", 2},
		{"a map literal is cut by element", `map[string]int{"a": 1, "b": 2}`, 2},
		{"a keyed literal of a named type is cut by element", "Pair{A: a, B: b}", 2},
		{"a positional literal of a named type stays whole", "Pair{a, b}", 0},
		{"a named slice written positionally stays whole", "Rules{a, b, c}", 0},
		{"a literal mixing keyed and positional stays whole", "Pair{a, B: b}", 0},
		{"a qualified named literal stays whole", "pkg.Pair{a, b}", 0},
		{"a slices.Concat call is cut by argument", "slices.Concat(a(), b())", 2},
		{"a slices.Concat spread stays whole", "slices.Concat(parts...)", 0},
		{"a call to anything else stays whole", "build(a, b)", 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := "package app\n\nfunc F() any {\n\treturn " + testCase.result + "\n}\n"

			file, err := parser.ParseFile(token.NewFileSet(), "app.go", source, 0)
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}

			body := file.Decls[0].(*ast.FuncDecl).Body

			elements, replace := ReturnedElements(body)
			if len(elements) != testCase.want {
				t.Fatalf("ReturnedElements() = %d elements, want %d", len(elements), testCase.want)
			}

			if isNil := replace == nil; isNil != (testCase.want == 0) {
				t.Fatalf("ReturnedElements() replace nil = %v, want %v", isNil, testCase.want == 0)
			}
		})
	}
}
