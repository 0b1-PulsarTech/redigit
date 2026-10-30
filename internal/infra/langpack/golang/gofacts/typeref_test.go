package gofacts

import (
	"context"
	"go/types"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type refCase struct {
	name   string
	object string
	want   facts.TypeRef
}

func refCases() []refCase {
	named := facts.TypeRef{Key: typeKey("Named"), Name: module + ".Named"}

	return []refCase{
		{"a package-scope named type carries its key", "Named", named},
		{
			"a pointer carries the key of its element",
			"Pointer",
			facts.TypeRef{Key: named.Key, Name: "*" + named.Name},
		},
		{
			"a slice carries the key of its element",
			"Slice",
			facts.TypeRef{Key: named.Key, Name: "[]" + named.Name},
		},
		{"a map has no key", "Mapping", facts.TypeRef{Name: "map[string]" + named.Name}},
		{"a builtin has no key", "Count", facts.TypeRef{Name: "int"}},
		{
			"a type local to a function keeps its name and has no key",
			"Local",
			facts.TypeRef{Name: module + ".Local"},
		},
	}
}

func TestRefsRef(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"a.go": `package fixture

type Named struct{}

var (
	Pointer *Named
	Slice   []Named
	Mapping map[string]Named
	Count   int
)

func holder() {
	type Local struct{}

	_ = Local{}
}
`,
	}

	fix := indexOf(t, files)

	roots, err := fix.source.Packages(context.Background(), fix.snap)
	if err != nil || len(roots) == 0 {
		t.Fatalf("Packages() = %d roots, error = %v", len(roots), err)
	}

	scope := refs{
		where: typed.NewPositions(fix.snap.Root, typed.NewDeclLocator(fix.decls)),
		fset:  roots[0].Fset,
	}

	for _, testCase := range refCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var found types.Object

			for ident, obj := range roots[0].TypesInfo.Defs {
				if obj != nil && ident.Name == testCase.object {
					found = obj
				}
			}

			if found == nil {
				t.Fatalf("%s is not declared in the fixture", testCase.object)
			}

			if got := scope.ref(found.Type()); got != testCase.want {
				t.Fatalf("ref(%s) = %+v, want %+v", testCase.object, got, testCase.want)
			}
		})
	}
}
