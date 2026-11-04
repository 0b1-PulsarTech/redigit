package selector

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestQualifier(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		imports     []string
		unit        entities.UnitID
		modules     map[string]bool
		want        []string
		wantMatched bool
	}{
		{
			name: "an aliased import answers to its alias and the base", unit: "internal/zlib",
			imports: []string{`z "example.com/p/internal/zlib"`},
			want:    []string{"z", "zlib"}, wantMatched: true,
		},
		{
			name: "a plain import answers to the directory base", unit: "internal/zlib",
			imports: []string{`"example.com/p/internal/zlib"`},
			want:    []string{"zlib"}, wantMatched: true,
		},
		{
			name:        "a module below the repository root still finds the alias",
			unit:        "backend/internal/zlib",
			imports:     []string{`z "example.com/p/internal/zlib"`},
			want:        []string{"z", "zlib"},
			wantMatched: true,
		},
		{
			name: "two imports sharing the directory base are both tried", unit: "errors",
			imports: []string{`stderrors "errors"`, `"example.com/p/errors"`},
			want:    []string{"errors", "stderrors"}, wantMatched: true,
		},
		{
			name: "the module root cannot be told from the imports so every one counts", unit: ".",
			imports: []string{`r "example.com/p"`, `"strings"`},
			want:    []string{"r", "strings"}, wantMatched: true,
		},
		{
			name:        "a test package unit answers to the directory it sits in",
			unit:        "internal/zlib#zlib_test",
			imports:     []string{`z "example.com/p/internal/zlib"`},
			want:        []string{"z", "zlib"},
			wantMatched: true,
		},
		{
			name: "no import names a plain package so none is read", unit: "backend",
			imports: []string{`r "example.com/p"`, `"strings"`},
		},
		{
			name: "a nested module root answers to every import", unit: "aaext",
			modules: map[string]bool{"aaext": true},
			imports: []string{`ext "example.com/ext"`, `"strings"`},
			want:    []string{"ext", "strings"}, wantMatched: true,
		},
		{
			name: "an import by dot or blank names nothing", unit: "internal/zlib",
			imports: []string{`. "example.com/p/internal/zlib"`, `_ "example.com/p/internal/zlib"`},
			want:    []string{"zlib"}, wantMatched: true,
		},
		{name: "no imports of a plain package name nothing", unit: "internal/zlib"},
		{name: "no imports at the root name nothing", unit: "."},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, isMatched := Names(testCase.imports, testCase.unit, testCase.modules)

			if !slices.Equal(got, testCase.want) || isMatched != testCase.wantMatched {
				t.Fatalf(
					"Names() = %v, %t, want %v, %t",
					got,
					isMatched,
					testCase.want,
					testCase.wantMatched,
				)
			}
		})
	}
}
