package typed

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

type escapeCase struct {
	name        string
	source      string
	target      string
	call        string
	readerFile  string
	wantMethods []string
}

const (
	pointerErr = "package a\n\ntype E struct{}\n\nfunc (*E) Error() string { return \"\" }\n"
	errorsAs   = "errors.As(err, &target)"
)

func TestEscapeReadsTheMethodsOfAnErrorsAsTarget(t *testing.T) {
	t.Parallel()

	valueErr := "package a\n\ntype E struct{}\n\nfunc (E) Error() string { return \"\" }\n"

	testCases := []escapeCase{
		{
			name:        "a pointer variable, a pointer-receiver Error",
			source:      pointerErr,
			target:      "*a.E",
			call:        errorsAs,
			wantMethods: []string{"E.Error"},
		},
		{
			name:        "a pointer variable, a value-receiver Error",
			source:      valueErr,
			target:      "*a.E",
			call:        errorsAs,
			wantMethods: []string{"E.Error"},
		},
		{
			name:        "a value variable, a value-receiver Error",
			source:      valueErr,
			target:      "a.E",
			call:        errorsAs,
			wantMethods: []string{"E.Error"},
		},
	}

	checkEscapeCases(t, testCases)
}

func TestEscapeReadsThePointeeBeyondErrorsAs(t *testing.T) {
	t.Parallel()

	decoder := "package a\n\ntype E struct{}\n\nfunc (*E) UnmarshalJSON([]byte) error { return nil }\n"

	testCases := []escapeCase{
		{
			name:        "a pointer variable handed to json.Unmarshal",
			source:      decoder,
			target:      "*a.E",
			call:        "json.Unmarshal(data, &target)",
			wantMethods: []string{"E.UnmarshalJSON"},
		},
		{
			name:        "three pointers deep",
			source:      pointerErr,
			target:      "**a.E",
			call:        errorsAs,
			wantMethods: []string{"E.Error"},
		},
		{
			name:        "an exported alias of a pointer type",
			source:      pointerErr + "\ntype Alias = *E\n",
			target:      "a.Alias",
			call:        errorsAs,
			wantMethods: []string{"E.Error"},
		},
		{
			name:        "a local alias of a pointer type",
			source:      pointerErr + "\ntype Alias = *E\n",
			target:      "Alias",
			call:        errorsAs,
			readerFile:  "a/read.go",
			wantMethods: []string{"E.Error"},
		},
		{
			name:   "a type without methods",
			source: "package a\n\ntype E struct{}\n",
			target: "*a.E",
			call:   errorsAs,
		},
	}

	checkEscapeCases(t, testCases)
}

func TestEscapeFindsTheReaderWhereverItLives(t *testing.T) {
	t.Parallel()

	testCases := []escapeCase{
		{
			name:        "a reader in a test file, the method in a non-test file",
			source:      pointerErr,
			target:      "*a.E",
			call:        errorsAs,
			readerFile:  "b/b_test.go",
			wantMethods: []string{"E.Error"},
		},
		{
			name:        "a reader in the package of the type",
			source:      pointerErr,
			target:      "*E",
			call:        errorsAs,
			readerFile:  "a/read.go",
			wantMethods: []string{"E.Error"},
		},
	}

	checkEscapeCases(t, testCases)
}

func checkEscapeCases(t *testing.T, testCases []escapeCase) {
	t.Helper()

	const readerSrc = "package %s\n\nimport (\n\t\"encoding/json\"\n\t\"errors\"%s\n)\n\n" +
		"var _, _ = errors.Is, json.Valid\n\n" +
		"func Read(err error, data []byte) error {\n\tvar target %s\n\n\treturn %s\n}\n"

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			readerFile := cmp.Or(testCase.readerFile, "b/b.go")
			readerPkg := path.Base(path.Dir(readerFile))
			imports := ""

			if readerPkg != "a" {
				imports = "\n\n\t\"example.test/fixture/a\""
			}

			snap, decls := testkit.Fixture(t, map[string]string{
				"a/a.go": testCase.source,
				readerFile: fmt.Sprintf(
					readerSrc, readerPkg, imports, testCase.target, testCase.call),
			})

			facts, err := NewSource(nil).Facts(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			gotMethods, gotOwner := escapedMethods(facts, "Read")
			if !slices.Equal(gotMethods, testCase.wantMethods) {
				t.Fatalf("Escaped = %+v, methods %v, want %v",
					facts.Escaped, gotMethods, testCase.wantMethods)
			}

			if len(gotMethods) > 0 && gotOwner != "E" {
				t.Fatalf("Owner = %q, want the type E", gotOwner)
			}
		})
	}
}

func TestEscapeSkipsATypesOwnMethodsOnlyWhenNoPointerToItIsTaken(t *testing.T) {
	t.Parallel()

	const (
		head = "package a\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\ntype E struct{}\n\n" +
			"var _, _ = errors.As, fmt.Sprint\n\n"
		errorPtr  = "func (*E) Error() string { return \"\" }\n\n"
		errorVal  = "func (E) Error() string { return \"\" }\n\n"
		unwrapped = "(err error) bool {\n\tvar target *E\n\n\treturn errors.As(err, &target)\n}\n"
	)

	testCases := []struct {
		name        string
		source      string
		dependent   string
		wantMethods []string
	}{
		{
			name:        "a pointer target declared in a pointer-receiver method",
			source:      head + errorPtr + "func (*E) Wrap" + unwrapped,
			dependent:   "E.Wrap",
			wantMethods: []string{"E.Error"},
		},
		{
			name:        "a pointer target declared in a value-receiver method",
			source:      head + errorPtr + "func (E) Wrap" + unwrapped,
			dependent:   "E.Wrap",
			wantMethods: []string{"E.Error"},
		},
		{
			name:      "the pointer receiver handed to an interface",
			source:    head + errorPtr + "func (e *E) Show() string { return fmt.Sprint(e) }\n",
			dependent: "E.Show",
		},
		{
			name:      "the value receiver handed to an interface",
			source:    head + errorVal + "func (e E) Show() string { return fmt.Sprint(e) }\n",
			dependent: "E.Show",
		},
		{
			name: "Error itself looking for a pointer target of its own type",
			source: head + "func (*E) Error() string {\n\tvar target *E\n\n" +
				"\t_ = errors.As(nil, &target)\n\n\treturn \"\"\n}\n",
			dependent: "E.Error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, map[string]string{"a/a.go": testCase.source})

			facts, err := NewSource(nil).Facts(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			gotMethods, _ := escapedMethods(facts, testCase.dependent)
			if !slices.Equal(gotMethods, testCase.wantMethods) {
				t.Fatalf("%s escapes %v, want %v",
					testCase.dependent, gotMethods, testCase.wantMethods)
			}
		})
	}
}

func escapedMethods(facts Facts, dependent string) (methods []string, owner string) {
	for _, req := range facts.Escaped {
		if req.Dependent.Name != dependent || req.Owner.Kind != entities.KindType {
			continue
		}

		owner = req.Owner.Name

		for _, method := range req.Methods {
			methods = append(methods, method.Name)
		}
	}

	slices.Sort(methods)

	return methods, owner
}
