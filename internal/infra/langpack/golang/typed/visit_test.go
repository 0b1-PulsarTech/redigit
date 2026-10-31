package typed

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestConversionVisitorSkipsACallTheCheckerNeverTyped(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
	}{
		{"a plain call", "package x\n\nfunc f() { g(1) }\n"},
		{"a call of a call", "package x\n\nfunc f() { g(1)(2) }\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			file, err := parser.ParseFile(token.NewFileSet(), "x.go", testCase.source, 0)
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}

			pkg := &packages.Package{TypesInfo: &types.Info{
				Types: map[ast.Expr]types.TypeAndValue{},
			}}
			scan := &factScan{pkg: pkg}

			ast.Walk(&conversionVisitor{scan: scan}, file)

			if len(scan.facts.Converted) != 0 {
				t.Fatalf("Converted = %+v, want none", scan.facts.Converted)
			}
		})
	}
}
