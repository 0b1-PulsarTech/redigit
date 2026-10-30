package resolve

import (
	"context"
	"maps"
	"slices"
	"testing"
)

func implementsFiles() map[string]string {
	return map[string]string{
		"a.go": `package fixture

type Namer interface{ Name() string }

type Setter interface{ Set() }

type Both interface {
	Namer
	Setter
}

type Anything interface{}

type Val struct{}

func (Val) Name() string { return "v" }

type Ptr struct{}

func (*Ptr) Name() string { return "p" }

func (*Ptr) Set() {}

type Box struct{ Val }

type Wrap struct{ Namer }

type Cell[T any] struct{ item T }

func (Cell[T]) Name() string { return "c" }

type Plain struct{}
`,
	}
}

func TestTypedImplementations(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, implementsFiles())

	testCases := []struct {
		name  string
		query spot
		want  []spot
	}{
		{
			"value receivers, pointer receivers, embedding and interfaces that extend it",
			spot{"a.go", "Namer interface", 0},
			[]spot{
				{"a.go", "Both interface", 0},
				{"a.go", "Val struct", 0},
				{"a.go", "Ptr struct", 0},
				{"a.go", "Box struct", 0},
				{"a.go", "Wrap struct", 0},
				{"a.go", "Cell[T any] struct", 0},
			},
		},
		{
			"only the type holding every method",
			spot{"a.go", "Both interface", 0},
			[]spot{{"a.go", "Ptr struct", 0}},
		},
		{"an empty interface names none", spot{"a.go", "Anything interface", 0}, nil},
		{"a struct is not an interface", spot{"a.go", "Val struct", 0}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.resolver.Implementations(
				context.Background(), fix.locate(t, testCase.query),
			)
			if err != nil {
				t.Fatalf("Implementations() error = %v", err)
			}

			want := fix.locations(t, testCase.want)
			slices.SortFunc(want, compareLocations)

			if !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Fatalf("Implementations() = %v, want %v", got, want)
			}
		})
	}
}

func TestTypedImplementationsOfAnAlias(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, map[string]string{"alias.go": `package fixture

type Namer interface{ String() string }

type Assistant = interface{ Assist() }

type Stasher[T any] = interface{ Stash(T) }

type Called = Namer

type assists struct{}

func (assists) Assist() {}

type stashes struct{}

func (stashes) Stash(int) {}

type callable struct{}

func (callable) String() string { return "" }
`})

	testCases := []struct {
		name  string
		query spot
		want  []spot
	}{
		{
			"an alias of an interface literal",
			spot{"alias.go", "Assistant =", 0},
			[]spot{{"alias.go", "assists struct", 0}},
		},
		{
			"a generic alias of an interface literal",
			spot{"alias.go", "Stasher[T any] =", 0},
			[]spot{{"alias.go", "stashes struct", 0}},
		},
		{
			"an alias of a named interface, which meets itself",
			spot{"alias.go", "Called =", 0},
			[]spot{{"alias.go", "Namer interface", 0}, {"alias.go", "callable struct", 0}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.resolver.Implementations(
				context.Background(), fix.locate(t, testCase.query),
			)
			if err != nil {
				t.Fatalf("Implementations() error = %v", err)
			}

			want := fix.locations(t, testCase.want)
			slices.SortFunc(want, compareLocations)

			if !slices.Equal(got, want) {
				t.Fatalf("Implementations() = %v, want %v", got, want)
			}
		})
	}
}

const genericSource = `package fixture

type User struct{}

type Repo[T any] interface{ Get(id string) (T, error) }

type userRepo struct{}

func (userRepo) Get(string) (User, error) { return User{}, nil }

type wrongRepo struct{}

func (wrongRepo) Get(int) (User, error) { return User{}, nil }

type Box[T any] struct{}

type Boxer[T any] interface{ Open() Box[T] }

type opener struct{}

func (opener) Open() Box[int] { return Box[int]{} }

type Stream[T any] interface{ Recv() <-chan T }

type feed struct{}

func (feed) Recv() <-chan int { return nil }

type Pair[T any] interface{ Both() [2]T }

type duo struct{}

func (duo) Both() [2]int { return [2]int{} }

type Swap[T any] interface{ Swap(T) T }

type mixed struct{}

func (mixed) Swap(int) string { return "" }

type Dup interface{ Dup(int, string) }

type Twin[T any] struct{}

func (Twin[T]) Dup(T, T) {}
`

const unifySource = `package fixture

type Folder[T any] interface{ Fold([]T, T) T }

type Summer[S any] struct{}

func (Summer[S]) Fold(S, S) S { var zero S; return zero }

type Folder2[T any] interface{ Fold([]T, T) }

type Summer2[S any] struct{}

func (Summer2[S]) Fold(S, S) {}

type Multi[T any] interface{ A(T); B() T }

type split struct{}

func (split) A(int) {}

func (split) B() string { return "" }

type Two interface{ A(int); B() string }

type Gen[S any] struct{}

func (Gen[S]) A(S) {}

func (Gen[S]) B() S { var zero S; return zero }

type Shape[T any] interface{ Form() struct{ V T } }

type shaper struct{}

func (shaper) Form() struct{ V int } { return struct{ V int }{} }

type Tagged[T any] interface{ Tag() struct{ V T "k" } }

type tagger struct{}

func (tagger) Tag() struct{ V int "k" } { return struct{ V int "k" }{} }

type retagger struct{}

func (retagger) Tag() struct{ V int "x" } { return struct{ V int "x" }{} }

type Taker[T any] interface{ Take(interface{ Label() T }) }

type taker struct{}

func (taker) Take(interface{ Label() int }) {}

type Stringish interface {
	~string
	Text() string
}

type MyInt int

func (MyInt) Text() string { return "" }

type MyStr string

func (MyStr) Text() string { return "" }
`

func goplsFiles() map[string]string {
	files := map[string]string{
		"cell.go": `package fixture

type Cell[T any] struct{ V T }

func (c Cell[T]) Get() T { return c.V }

type Getter interface{ Get() int }

type Putter interface{ Put(int) }
`,
		"plain.go": "package fixture\n\ntype Plain struct{}\n\nfunc (Plain) Get() int { return 1 }\n",
		"gen.go": `package fixture

type Other[T any] struct{}

func (Other[T]) Get() string { return "" }

func (Other[T]) Put(x T) {}

type Slice[T any] struct{}

func (Slice[T]) Get() []T { return nil }

func Local() { type inner struct{ Plain }; _ = inner{} }
`,
		"embed.go": "package fixture\n\ntype Namer interface{ String() string }\n\n" +
			"type Stringy[T any] interface{ Take(interface{ Namer; Label() T }) }\n\n" +
			"type stringyInt struct{}\n\n" +
			"func (stringyInt) Take(interface{ Namer; Label() int }) {}\n",
		"generic.go": genericSource,
		"unify.go":   unifySource,
		"doc.go":     "package fixture\n",
		"sub/user.go": `package sub

import "example.test/fixture"

func F() {
	type local struct{ fixture.Plain }
	_ = local{}
}
`,
		"sub/named.go":     "package sub\n\nimport fx \"example.test/fixture\"\n\nvar _ fx.Plain\n",
		"sub/blank.go":     "package sub\n\nimport _ \"example.test/fixture\"\n",
		"pk/pk.go":         "package pk\n",
		"pk/in_test.go":    "package pk\n",
		"pk/x_test.go":     "package pk_test\n\nimport _ \"example.test/fixture/pk\"\n",
		"pkuser/user.go":   "package pkuser\n\nimport _ \"example.test/fixture/pk\"\n",
		"pkuser/x_test.go": "package pkuser_test\n\nimport _ \"example.test/fixture/pk\"\n",
	}

	maps.Copy(files, variantFiles())

	return files
}

func variantFiles() map[string]string {
	return map[string]string{
		"ta/ta.go":      "package ta\n\ntype X struct{}\n",
		"ta/ta_test.go": "package ta\n\ntype Getter interface{ Get() X }\n",
		"ta/x_test.go":  "package ta_test\n\nimport _ \"example.test/fixture/tas\"\n",
		"tar/r.go": "package tar\n\nimport \"example.test/fixture/ta\"\n\ntype T struct{}\n\n" +
			"func (T) Get() ta.X { return ta.X{} }\n",
		"tas/s.go": "package tas\n\nimport \"example.test/fixture/ta\"\n\ntype U struct{}\n\n" +
			"func (U) Get() ta.X { return ta.X{} }\n",
		"tan/n.go": "package tan\n\ntype X struct{}\n\ntype N struct{}\n\n" +
			"func (N) Get() X { return X{} }\n",
		"tb/tb.go": "package tb\n\ntype X struct{}\n",
		"tb/tb_test.go": "package tb\n\ntype fake struct{}\n\n" +
			"func (fake) Get() X { return X{} }\n",
		"tbp/port.go": "package tbp\n\nimport \"example.test/fixture/tb\"\n\n" +
			"type Port interface{ Get() tb.X }\n",
	}
}

func variantGoplsCases() []goplsCase {
	return []goplsCase{
		{
			"an interface only a test variant holds is met by a package no test of it recompiles",
			spot{"ta/ta_test.go", "Getter interface", 0},
			[]spot{
				{"cell.go", "Cell[T any]", 0},
				{"tar/r.go", "T struct", 0},
				{"tas/s.go", "U struct", 0},
			},
		},
		{
			"a type only a test variant holds meets an interface of a package outside it",
			spot{"tbp/port.go", "Port interface", 0},
			[]spot{{"cell.go", "Cell[T any]", 0}, {"tb/tb_test.go", "fake struct", 0}},
		},
	}
}

type goplsCase struct {
	name  string
	query spot
	want  []spot
}

func unifyGoplsCases() []goplsCase {
	return []goplsCase{
		{
			"a parameter bound to a type that contains it, with a result",
			spot{"unify.go", "Folder[T any]", 0},
			nil,
		},
		{
			"a parameter bound to a type that contains it, without a result",
			spot{"unify.go", "Folder2[T any]", 0},
			nil,
		},
		{
			"a type parameter bound differently by two methods",
			spot{"unify.go", "Multi[T any]", 0},
			[]spot{
				{"unify.go", "split struct", 0},
				{"unify.go", "Two interface", 0},
				{"unify.go", "Gen[S any]", 0},
			},
		},
		{
			"a concrete interface against a type parameter bound per method",
			spot{"unify.go", "Two interface", 0},
			[]spot{
				{"unify.go", "Multi[T any]", 0},
				{"unify.go", "split struct", 0},
				{"unify.go", "Gen[S any]", 0},
			},
		},
		{
			"a struct literal in a signature",
			spot{"unify.go", "Shape[T any]", 0},
			[]spot{{"unify.go", "shaper struct", 0}},
		},
		{
			"a struct literal with a tag",
			spot{"unify.go", "Tagged[T any]", 0},
			[]spot{{"unify.go", "tagger struct", 0}},
		},
		{
			"an interface literal in a signature",
			spot{"unify.go", "Taker[T any]", 0},
			[]spot{{"unify.go", "taker struct", 0}},
		},
		{
			"an interface literal embedding another, a type parameter in a method of it",
			spot{"embed.go", "Stringy[T any]", 0},
			[]spot{{"embed.go", "stringyInt struct", 0}},
		},
		{
			"a constraint interface with methods",
			spot{"unify.go", "Stringish interface", 0},
			[]spot{{"unify.go", "MyInt int", 0}, {"unify.go", "MyStr string", 0}},
		},
	}
}

func TestTypedImplementationsMatchGopls(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, goplsFiles())

	testCases := []goplsCase{
		{
			"a type parameter stands for any type, and a local type of the same package counts",
			spot{"cell.go", "Getter interface", 0},
			[]spot{
				{"cell.go", "Cell[T any]", 0},
				{"gen.go", "inner struct", 0},
				{"plain.go", "Plain struct", 0},
			},
		},
		{
			"a type parameter in a parameter position",
			spot{"cell.go", "Putter interface", 0},
			[]spot{{"gen.go", "Other[T any]", 0}},
		},
		{
			"a generic interface is implemented by the type fixing its parameter",
			spot{"generic.go", "Repo[T any]", 0},
			[]spot{{"generic.go", "userRepo struct", 0}},
		},
		{
			"an instantiated named type argument",
			spot{"generic.go", "Boxer[T any]", 0},
			[]spot{{"generic.go", "opener struct", 0}},
		},
		{
			"a channel element",
			spot{"generic.go", "Stream[T any]", 0},
			[]spot{{"generic.go", "feed struct", 0}},
		},
		{
			"an array element",
			spot{"generic.go", "Pair[T any]", 0},
			[]spot{{"generic.go", "duo struct", 0}},
		},
		{
			"one type parameter bound to two different types",
			spot{"generic.go", "Swap[T any]", 0},
			nil,
		},
		{
			"one candidate type parameter bound to two different types",
			spot{"generic.go", "Dup interface", 0},
			nil,
		},
	}

	for _, testCase := range slices.Concat(testCases, unifyGoplsCases(), variantGoplsCases()) {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.resolver.Implementations(
				context.Background(), fix.locate(t, testCase.query),
			)
			if err != nil {
				t.Fatalf("Implementations() error = %v", err)
			}

			want := fix.locations(t, testCase.want)
			slices.SortFunc(want, compareLocations)

			if !slices.Equal(got, want) {
				t.Fatalf("Implementations() = %v, want %v", got, want)
			}
		})
	}
}
