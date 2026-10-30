package resolve

import (
	"go/token"
	"go/types"
	"slices"
	"testing"
)

func TestParts(t *testing.T) {
	t.Parallel()

	member, other := types.Typ[types.Int], types.Typ[types.String]
	label := testSignature([]types.Type{member}, other)
	field := types.NewField(token.NoPos, nil, "V", member, false)

	testCases := []struct {
		name string
		typ  types.Type
		want []types.Type
	}{
		{"a pointer", types.NewPointer(member), []types.Type{member}},
		{"a slice", types.NewSlice(member), []types.Type{member}},
		{"an array", types.NewArray(member, 2), []types.Type{member}},
		{"a channel", types.NewChan(types.SendRecv, member), []types.Type{member}},
		{"a map", types.NewMap(member, other), []types.Type{member, other}},
		{"a signature", label, []types.Type{member, other}},
		{"a struct", types.NewStruct([]*types.Var{field}, nil), []types.Type{member}},
		{
			"an interface",
			testInterface("Label", label),
			[]types.Type{label},
		},
		{"a basic type", member, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := parts(testCase.typ); !slices.Equal(got, testCase.want) {
				t.Fatalf("parts() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestUnifierSameDeclaration(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	first, second := fset.AddFile("a.go", -1, 100), fset.AddFile("b.go", -1, 100)
	pkg, variant, other := types.NewPackage("p", "p"), types.NewPackage("p", "p"),
		types.NewPackage("q", "q")
	declared := func(file *token.File, offset int, owner *types.Package, name string) *types.TypeName {
		return types.NewTypeName(file.Pos(offset), owner, name, nil)
	}
	site := declared(first, 10, pkg, "X")

	testCases := []struct {
		name        string
		fset        *token.FileSet
		left, right *types.TypeName
		want        bool
	}{
		{"one object", fset, site, site, true},
		{"a recompiled copy of it", fset, site, declared(first, 10, variant, "X"), true},
		{"the same name in another package", fset, site, declared(first, 10, other, "X"), false},
		{"another name at the same site", fset, site, declared(first, 10, variant, "Y"), false},
		{"another offset", fset, site, declared(first, 20, variant, "X"), false},
		{"another file", fset, site, declared(second, 10, variant, "X"), false},
		{"no position", fset, site, types.NewTypeName(token.NoPos, variant, "X", nil), false},
		{"no package", fset, site, types.NewTypeName(first.Pos(10), nil, "X", nil), false},
		{"no file set", nil, site, declared(first, 10, variant, "X"), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := newUnifier(testCase.fset).sameDeclaration(testCase.left, testCase.right)
			if got != testCase.want {
				t.Fatalf("sameDeclaration() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestUnifierMentions(t *testing.T) {
	t.Parallel()

	param, other := testParam("T"), testParam("S")
	deep := types.Type(param)

	for range maxUnifyDepth + 1 {
		deep = types.NewPointer(deep)
	}

	testCases := []struct {
		name  string
		typ   types.Type
		bound types.Type
		want  bool
	}{
		{"the parameter itself", param, nil, true},
		{"another parameter", other, nil, false},
		{"a slice of the parameter", types.NewSlice(param), nil, true},
		{"a map keyed by another type", types.NewMap(types.Typ[types.Int], other), nil, false},
		{"another parameter bound to a type holding it", other, types.NewSlice(param), true},
		{"a nesting past the depth guard", deep, nil, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			uni := newUnifier(nil)
			if testCase.bound != nil {
				uni.bound[other] = testCase.bound
			}

			if got := uni.mentions(param, testCase.typ, 0); got != testCase.want {
				t.Fatalf("mentions() = %v, want %v", got, testCase.want)
			}
		})
	}
}
