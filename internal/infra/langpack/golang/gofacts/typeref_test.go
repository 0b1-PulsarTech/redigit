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

func TestRefsRefOfAliases(t *testing.T) {
	t.Parallel()

	named := facts.TypeRef{Key: typeKey("Named"), Name: module + ".Named"}
	testCases := []refCase{
		{"a package-scope named type carries its key", "Named", named},
		{"an alias is the type it stands for", "Aliased", named},
		{
			"a pointer to an alias names the type under it",
			"AliasedPtr",
			facts.TypeRef{Key: named.Key, Name: "*" + named.Name},
		},
		{
			"a map of an alias names the type under it",
			"AliasedMap",
			facts.TypeRef{Name: "map[string]" + named.Name},
		},
		{
			"a function names the types under its aliased parameters and results",
			"AliasedFunc",
			facts.TypeRef{
				Name: "func(" + named.Name + ", ..." + named.Name + ") (*" + named.Name + ", error)",
			},
		},
		{
			"a generic instance names the types under its aliased arguments",
			"AliasedBox",
			facts.TypeRef{Key: typeKey("Box"), Name: module + ".Box[" + named.Name + "]"},
		},
		{
			"a generic instance unaliases every argument",
			"AliasedPair",
			facts.TypeRef{
				Key:  typeKey("Pair"),
				Name: module + ".Pair[" + named.Name + ", *" + named.Name + "]",
			},
		},
	}

	requireRefs(t, testCases)
}

func TestRefsRef(t *testing.T) {
	t.Parallel()

	named := facts.TypeRef{Key: typeKey("Named"), Name: module + ".Named"}
	testCases := []refCase{
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

	requireRefs(t, testCases)
}

func requireRefs(t *testing.T, testCases []refCase) {
	t.Helper()

	files := map[string]string{
		"a.go": `package fixture

type Named struct{}

type Alias = Named

type Box[Item any] struct{}

type Pair[Left, Right any] struct{}

var (
	Aliased    Alias
	AliasedPtr *Alias
	AliasedMap map[string]Alias
	AliasedFunc func(Alias, ...Alias) (*Alias, error)
	AliasedBox  Box[Alias]
	AliasedPair Pair[Alias, *Alias]
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
		where: typed.NewPositions(fix.snap.Root, typed.NewDeclLocator(fix.decls)).Parsing(roots),
		fset:  roots[0].Fset,
	}

	for _, testCase := range testCases {
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
