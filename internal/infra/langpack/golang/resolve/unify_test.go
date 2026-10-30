package resolve

import (
	"go/token"
	"go/types"
	"slices"
	"testing"
)

func testInterface(
	name string,
	signature *types.Signature,
	embedded ...types.Type,
) *types.Interface {
	var methods []*types.Func
	if name != "" {
		methods = append(methods, types.NewFunc(token.NoPos, nil, name, signature))
	}

	return types.NewInterfaceType(methods, embedded).Complete()
}

func testVariadic(member types.Type) *types.Signature {
	return types.NewSignatureType(
		nil, nil, nil, testTuple(types.NewSlice(member)), testTuple(), true,
	)
}

func unifyShapeRows() []unifyRow {
	param, intType, text := testParam("T"), types.Typ[types.Int], types.Typ[types.String]
	label := func(member types.Type) *types.Signature { return testSignature(nil, member) }
	union := func(member types.Type) types.Type {
		return types.NewUnion([]*types.Term{types.NewTerm(false, member)})
	}

	return []unifyRow{
		{
			"an interface method bound to a type",
			testInterface("Label", label(param)), testInterface("Label", label(intType)), true,
		},
		{
			"an interface method of another name",
			testInterface("Label", label(param)), testInterface("Other", label(intType)), false,
		},
		{
			"an interface with more methods",
			testInterface("Label", label(param)), testInterface("", nil), false,
		},
		{
			"an interface against a pointer",
			testInterface("Label", label(param)), types.NewPointer(intType), false,
		},
		{
			"interfaces embedding the same term",
			testInterface("", nil, union(intType)), testInterface("", nil, union(intType)), true,
		},
		{
			"interfaces embedding different terms",
			testInterface("", nil, union(intType)), testInterface("", nil, union(text)), false,
		},
		{
			"a map bound on both sides",
			types.NewMap(param, param), types.NewMap(intType, intType), true,
		},
		{
			"a map bound to two types",
			types.NewMap(param, param), types.NewMap(intType, text), false,
		},
		{"a pointer bound to a type", types.NewPointer(param), types.NewPointer(intType), true},
		{"a pointer against a slice", types.NewPointer(param), types.NewSlice(intType), false},
		{"a slice bound to a type", types.NewSlice(param), types.NewSlice(intType), true},
		{"a slice against a pointer", types.NewSlice(param), types.NewPointer(intType), false},
		{"arrays of one length", types.NewArray(param, 2), types.NewArray(intType, 2), true},
		{"arrays of two lengths", types.NewArray(param, 2), types.NewArray(intType, 3), false},
		{
			"channels of one direction",
			types.NewChan(types.SendRecv, param), types.NewChan(types.SendRecv, intType), true,
		},
		{
			"channels of two directions",
			types.NewChan(types.SendRecv, param), types.NewChan(types.RecvOnly, intType), false,
		},
		{"variadic against variadic", testVariadic(param), testVariadic(intType), true},
		{
			"variadic against a slice parameter",
			testVariadic(param), testSignature([]types.Type{types.NewSlice(intType)}), false,
		},
	}
}

func testParam(name string) *types.TypeParam {
	return types.NewTypeParam(
		types.NewTypeName(token.NoPos, nil, name, nil), types.NewInterfaceType(nil, nil),
	)
}

func testTuple(members ...types.Type) *types.Tuple {
	vars := make([]*types.Var, 0, len(members))
	for _, member := range members {
		vars = append(vars, types.NewVar(token.NoPos, nil, "", member))
	}

	return types.NewTuple(vars...)
}

func testSignature(params []types.Type, results ...types.Type) *types.Signature {
	return types.NewSignatureType(nil, nil, nil, testTuple(params...), testTuple(results...), false)
}

func testStruct(name, tag string, embedded bool, member types.Type) *types.Struct {
	return types.NewStruct(
		[]*types.Var{types.NewField(token.NoPos, nil, name, member, embedded)}, []string{tag},
	)
}

type unifyRow struct {
	name  string
	left  types.Type
	right types.Type
	want  bool
}

func unifyRows() []unifyRow {
	param, other := testParam("T"), testParam("S")
	intType := types.Typ[types.Int]

	deep := func(base types.Type, levels int) types.Type {
		for range levels {
			base = types.NewPointer(base)
		}

		return base
	}

	return []unifyRow{
		{"the same parameter", param, param, true},
		{"two parameters", param, other, true},
		{"a parameter against a type", param, intType, true},
		{"a parameter against a slice of itself", param, types.NewSlice(param), false},
		{"a slice of a parameter against the parameter", types.NewSlice(param), param, false},
		{
			"a parameter bound to a type holding it, with a result",
			testSignature([]types.Type{types.NewSlice(param), param}, param),
			testSignature([]types.Type{other, other}, other),
			false,
		},
		{
			"a parameter bound to a type holding it, without a result",
			testSignature([]types.Type{types.NewSlice(param), param}),
			testSignature([]types.Type{other, other}),
			false,
		},
		{
			"a parameter bound once and used again",
			testSignature([]types.Type{param, param}),
			testSignature([]types.Type{intType, intType}),
			true,
		},
		{
			"a parameter bound to two types",
			testSignature([]types.Type{param, param}),
			testSignature([]types.Type{intType, types.Typ[types.String]}),
			false,
		},
		{
			"a struct field bound to a type",
			testStruct("V", "", false, param),
			testStruct("V", "", false, intType),
			true,
		},
		{
			"a struct field of another name",
			testStruct("V", "", false, param),
			testStruct("W", "", false, intType),
			false,
		},
		{
			"a struct field of another tag",
			testStruct("V", "k", false, param),
			testStruct("V", "x", false, intType),
			false,
		},
		{
			"a struct field embedded in one",
			testStruct("V", "", true, param),
			testStruct("V", "", false, intType),
			false,
		},
		{
			"a struct against a pointer",
			testStruct("V", "", false, param),
			types.NewPointer(intType),
			false,
		},
		{"a shallow nesting", deep(param, 8), deep(intType, 8), true},
		{"a nesting past the depth guard", deep(param, 200), deep(intType, 200), false},
	}
}

func TestUnifierMatch(t *testing.T) {
	t.Parallel()

	for _, testCase := range slices.Concat(unifyRows(), unifyShapeRows()) {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := newUnifier(nil).match(testCase.left, testCase.right); got != testCase.want {
				t.Fatalf("match() = %v, want %v", got, testCase.want)
			}
		})
	}
}
