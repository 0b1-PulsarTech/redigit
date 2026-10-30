package gofacts

import (
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func callsFiles() map[string]string {
	return map[string]string{
		"a.go": `package fixture

import (
	"fmt"
	"strings"
)

const prefix = "pre"

type Repo struct{}

func (Repo) Get(name string) string { return name }

func (*Repo) Put(name string, apply func(string)) {}

type Reader interface{ Read() int }

type Box[T any] struct{ item T }

func (Box[T]) Get() T { var zero T; return zero }

func useBoxMethod(box Box[int]) { box.Get() }

func Make[T any](item T) Box[T] { return Box[T]{item: item} }

func Pair[K comparable, V any](key K, value V) {}

func handler(string) {}

func take(string) {}

func apply(func(string) string) {}

func Plain() {}

func usePlain() { Plain() }

func useValue(repo Repo) { repo.Get("x") }

func usePointer() {
	repo := &Repo{}
	repo.Put("k", handler)
}

func useExplicit() { Pair[string, int]("k", 1) }

func useInferred() { Make(Repo{}) }

func useFuncValue() { apply(strings.ToUpper); take(prefix) }

func useMethodValue(repo Repo) { apply(repo.Get) }

func useClosure() {
	run := func() { Plain() }
	run()
}

func useExternal() { fmt.Sprint("q"); _ = strings.ToUpper("a") }

func useInterface(reader Reader) { reader.Read() }

func run(func() int) {}
func useInterfaceValue(reader Reader) { run(reader.Read) }

type Hook struct{ fire func() }
func useFuncVar(handler func(string)) { handler("x") }
func useFuncField(hook Hook) { hook.fire() }

func useBuiltins(values []int) int { return len(values) + int(int64(1)) }

func useNested() { take(fmt.Sprint(prefix)) }

func useLocalType() {
	type Local struct{}

	Make(Local{})
}
`,
	}
}

type callsCase struct {
	name string
	site string
	want []facts.Call
}

func TestIndexCalls(t *testing.T) {
	t.Parallel()

	fix := indexOf(t, callsFiles())

	for _, testCase := range slices.Concat(callsCases(), argumentCases()) {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := fix.index.Calls(funcKey("a.go", testCase.site))
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("Calls(%s) = %+v, want %+v", testCase.site, got, testCase.want)
			}
		})
	}
}

func callsCases() []callsCase {
	plain := funcKey("a.go", "Plain")
	repo := typeKey("Repo")
	repoRef := facts.TypeRef{Key: repo, Name: module + ".Repo"}
	get := methodKey("Repo.Get")

	return []callsCase{
		{
			"a plain call",
			"usePlain",
			[]facts.Call{
				{Site: funcKey("a.go", "usePlain"), Callee: module + ".Plain", CalleeKey: plain},
			},
		},
		{
			"a value method call with a constant argument",
			"useValue",
			[]facts.Call{{
				Site: funcKey("a.go", "useValue"), Callee: module + ".Repo.Get",
				CalleeKey: get, Receiver: repoRef, Literals: []string{"x"},
			}},
		},
		{
			"a pointer method call with a func value argument",
			"usePointer",
			[]facts.Call{{
				Site: funcKey("a.go", "usePointer"), Callee: module + ".Repo.Put",
				CalleeKey: methodKey("Repo.Put"), Receiver: repoRef,
				FuncArgs: []entities.DeclKey{funcKey("a.go", "handler")}, Literals: []string{"k"},
			}},
		},
		{
			"explicit type arguments",
			"useExplicit",
			[]facts.Call{{
				Site: funcKey("a.go", "useExplicit"), Callee: module + ".Pair",
				CalleeKey: funcKey("a.go", "Pair"), Literals: []string{"k"},
				TypeArgs: []facts.TypeRef{{Name: "string"}, {Name: "int"}},
			}},
		},
		{
			"inferred type arguments",
			"useInferred",
			[]facts.Call{{
				Site: funcKey("a.go", "useInferred"), Callee: module + ".Make",
				CalleeKey: funcKey("a.go", "Make"), TypeArgs: []facts.TypeRef{repoRef},
			}},
		},
		{
			"a method of an instantiated generic type takes the receiver's type arguments",
			"useBoxMethod",
			[]facts.Call{{
				Site: funcKey("a.go", "useBoxMethod"), Callee: module + ".Box.Get",
				CalleeKey: methodKey("Box.Get"),
				Receiver:  facts.TypeRef{Key: typeKey("Box"), Name: module + ".Box"},
				TypeArgs:  []facts.TypeRef{{Name: "int"}},
			}},
		},
		{
			"an interface method value is no func argument",
			"useInterfaceValue",
			[]facts.Call{{
				Site: funcKey("a.go", "useInterfaceValue"), Callee: module + ".run",
				CalleeKey: funcKey("a.go", "run"),
			}},
		},
		{
			"a type declared inside a function has a name and no key",
			"useLocalType",
			[]facts.Call{{
				Site: funcKey("a.go", "useLocalType"), Callee: module + ".Make",
				CalleeKey: funcKey("a.go", "Make"),
				TypeArgs:  []facts.TypeRef{{Name: module + ".Local"}},
			}},
		},
	}
}

func argumentCases() []callsCase {
	plain := funcKey("a.go", "Plain")
	get := methodKey("Repo.Get")

	return []callsCase{
		{
			"an external func value is not a key and a constant is a literal",
			"useFuncValue",
			[]facts.Call{
				{
					Site: funcKey("a.go", "useFuncValue"), Callee: module + ".apply",
					CalleeKey: funcKey("a.go", "apply"),
				},
				{
					Site: funcKey("a.go", "useFuncValue"), Callee: module + ".take",
					CalleeKey: funcKey("a.go", "take"), Literals: []string{"pre"},
				},
			},
		},
		{
			"a method value argument",
			"useMethodValue",
			[]facts.Call{{
				Site: funcKey("a.go", "useMethodValue"), Callee: module + ".apply",
				CalleeKey: funcKey("a.go", "apply"), FuncArgs: []entities.DeclKey{get},
			}},
		},
		{
			"a call inside a closure belongs to the declaration holding it",
			"useClosure",
			[]facts.Call{
				{Site: funcKey("a.go", "useClosure"), Callee: module + ".Plain", CalleeKey: plain},
			},
		},
		{
			"an external callee has no key and a qualified name",
			"useExternal",
			[]facts.Call{
				{
					Site:     funcKey("a.go", "useExternal"),
					Callee:   "fmt.Sprint",
					Literals: []string{"q"},
				},
				{
					Site:     funcKey("a.go", "useExternal"),
					Callee:   "strings.ToUpper",
					Literals: []string{"a"},
				},
			},
		},
		{
			"a call on an interface value",
			"useInterface",
			[]facts.Call{{
				Site: funcKey("a.go", "useInterface"), Callee: module + ".Reader.Read",
				Receiver: facts.TypeRef{Key: typeKey("Reader"), Name: module + ".Reader"},
			}},
		},
		{"builtins and conversions name no callee", "useBuiltins", nil},
		{"a call through a func-typed parameter is no call", "useFuncVar", nil},
		{"a call through a func-typed field is no call", "useFuncField", nil},
		{
			"a call nested in an argument follows its caller",
			"useNested",
			[]facts.Call{
				{
					Site: funcKey("a.go", "useNested"), Callee: module + ".take",
					CalleeKey: funcKey("a.go", "take"),
				},
				{
					Site: funcKey("a.go", "useNested"), Callee: "fmt.Sprint",
					Literals: []string{"pre"},
				},
			},
		},
		{"a declaration that calls nothing", "Plain", nil},
	}
}

func TestIndexCallsNameTheDeclaringType(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"a.go": `package fixture

type Namer interface{ Name() string }

type Ext interface{ Namer }

func viaExt(ext Ext) { ext.Name() }

type Inner struct{}

func (*Inner) Hi() {}

type Outer struct{ *Inner }

func viaOuter(outer Outer) { outer.Hi() }
`,
	}

	fix := indexOf(t, files)

	testCases := []callsCase{
		{
			"a method of an embedded interface names the interface declaring it",
			"viaExt",
			[]facts.Call{{
				Site: funcKey("a.go", "viaExt"), Callee: module + ".Namer.Name",
				Receiver: facts.TypeRef{Key: typeKey("Namer"), Name: module + ".Namer"},
			}},
		},
		{
			"a method promoted through an embedded pointer names the embedded type",
			"viaOuter",
			[]facts.Call{{
				Site: funcKey("a.go", "viaOuter"), Callee: module + ".Inner.Hi",
				CalleeKey: methodKey("Inner.Hi"),
				Receiver:  facts.TypeRef{Key: typeKey("Inner"), Name: module + ".Inner"},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := fix.index.Calls(funcKey("a.go", testCase.site))
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("Calls(%s) = %+v, want %+v", testCase.site, got, testCase.want)
			}
		})
	}
}

func TestIndexCallsOnInterfaces(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"a.go": `package fixture

import "io"

func Name() {}

type Namer interface{ Name() string }

type Anon = interface{ Name() string }

func useAlias(value Anon) { value.Name() }

func useStructLiteral() { struct{ io.Reader }{}.Read(nil) }

func useAnonymous[T interface{ Name() string }](value T) { value.Name() }

func useTilde[T interface {
	~int
	Name() string
}](value T) {
	value.Name()
}

func useAssertion(value any) {
	if named, ok := value.(interface{ Name() string }); ok {
		named.Name()
	}
}

func useNamedConstraint[T Namer](value T) { value.Name() }
`,
	}

	fix := indexOf(t, files)

	testCases := []callsCase{
		{"a call on an anonymous constraint is no call", "useAnonymous", nil},
		{"a call on a constraint with a core type is no call", "useTilde", nil},
		{"a call on an inline asserted interface is no call", "useAssertion", nil},
		{"a call through an alias of an anonymous interface is no call", "useAlias", nil},
		{
			"a method promoted from an embedded interface of a struct literal",
			"useStructLiteral",
			[]facts.Call{{
				Site: funcKey("a.go", "useStructLiteral"), Callee: "io.Reader.Read",
				Receiver: facts.TypeRef{Name: "io.Reader"},
			}},
		},
		{
			"a call on a named constraint keeps its interface",
			"useNamedConstraint",
			[]facts.Call{{
				Site: funcKey("a.go", "useNamedConstraint"), Callee: module + ".Namer.Name",
				Receiver: facts.TypeRef{Key: typeKey("Namer"), Name: module + ".Namer"},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := fix.index.Calls(funcKey("a.go", testCase.site))
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("Calls(%s) = %+v, want %+v", testCase.site, got, testCase.want)
			}
		})
	}
}

func TestIndexCallsAttributesToTheStatement(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"a.go": "package fixture\n\nfunc Plain() {}\n\nfunc take(string) {}\n\n" +
			"func wire() {\n\tPlain()\n\ttake(\"s\")\n}\n",
	}

	snap, decls := testkit.Fixture(t, files)
	wire := funcKey("a.go", "wire")
	first := testkit.DeclKey("a.go", entities.KindStmt, "wire#0")
	second := testkit.DeclKey("a.go", entities.KindStmt, "wire#1")
	cut := cutStatements(t, decls, files["a.go"], wire, "Plain()", "take(\"s\")")

	fix := indexFor(t, snap, cut)

	testCases := []struct {
		name string
		site entities.DeclKey
		want []facts.Call
	}{
		{
			"the first statement holds its own call",
			first,
			[]facts.Call{
				{Site: first, Callee: module + ".Plain", CalleeKey: funcKey("a.go", "Plain")},
			},
		},
		{
			"the second statement holds its own call",
			second,
			[]facts.Call{{
				Site: second, Callee: module + ".take",
				CalleeKey: funcKey("a.go", "take"), Literals: []string{"s"},
			}},
		},
		{"the head holds none", wire, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := fix.Calls(testCase.site); !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("Calls(%s) = %+v, want %+v", testCase.site, got, testCase.want)
			}
		})
	}
}
