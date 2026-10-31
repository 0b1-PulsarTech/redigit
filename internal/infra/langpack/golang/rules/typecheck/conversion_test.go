package typecheck

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

const conversionTypes = `package fixture

type Reader interface{ Read() string }

type Repo struct{}

func (Repo) Read() string { return "" }

func (Repo) Write(string) {}
`

func TestTypedConversionRule(t *testing.T) {
	t.Parallel()

	read := testkit.DeclKey("types.go", entities.KindMethod, "Repo.Read")

	testCases := []struct {
		name   string
		use    string
		wantBy entities.DeclKey
	}{
		{
			"an argument handed to an interface parameter",
			"func Take(Reader) {}\n\nfunc Wire() { Take(Repo{}) }\n",
			testkit.DeclKey("use.go", entities.KindFunc, "Wire"),
		},
		{
			"a return of a concrete value from an interface result",
			"func New() Reader { return &Repo{} }\n",
			testkit.DeclKey("use.go", entities.KindFunc, "New"),
		},
		{
			"a blank assertion",
			"var _ Reader = (*Repo)(nil)\n",
			testkit.DeclKey("use.go", entities.KindValue, "_ Reader = (*Repo)(nil)"),
		},
		{
			"a field of a composite literal",
			"type Holder struct{ R Reader }\n\nvar Held = Holder{R: Repo{}}\n",
			testkit.DeclKey("use.go", entities.KindValue, "Held"),
		},
		{
			"an assignment to an interface variable",
			"func Set() { var r Reader; r = Repo{}; _ = r }\n",
			testkit.DeclKey("use.go", entities.KindFunc, "Set"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, map[string]string{
				"types.go": conversionTypes,
				"use.go":   "package fixture\n\n" + testCase.use,
			})

			edges, err := Conversion{Source: typed.NewSource(nil)}.Edges(
				context.Background(), snap, decls,
			)
			if err != nil {
				t.Fatalf("Edges() error = %v", err)
			}

			want := entities.Edge{
				Dependent: testCase.wantBy,
				Provider:  read,
				Reason:    entities.ReasonImplements,
			}

			got := testkit.EdgeSet(edges)
			if !got[want] {
				t.Fatalf("Edges() = %+v, missing %+v", edges, want)
			}

			// Only what the interface lists is needed: Write is not.
			for edge := range got {
				if edge.Provider.Name == "Repo.Write" {
					t.Fatalf("Edges() waits for a method the interface does not list: %+v", edge)
				}
			}
		})
	}
}

func TestTypedConversionRuleIgnoresInterfaceToInterface(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"types.go": conversionTypes,
		"use.go": `package fixture

type Wide interface {
	Reader
	Write(string)
}

func Narrow(w Wide) Reader { return w }
`,
	})

	edges, err := Conversion{Source: typed.NewSource(nil)}.Edges(
		context.Background(), snap, decls,
	)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("Edges() = %+v, want none: an interface owns no method to wait for", edges)
	}
}

type factForm struct {
	name      string
	types     string
	use       string
	dependent entities.DeclKey
	provider  entities.DeclKey
	none      bool
}

func useFunc(name string) entities.DeclKey {
	return testkit.DeclKey("use.go", entities.KindFunc, name)
}

func TestTypedConversionRuleBuiltins(t *testing.T) {
	t.Parallel()

	read := testkit.DeclKey("types.go", entities.KindMethod, "Repo.Read")
	use := useFunc

	testCases := []factForm{
		{
			name:      "append of a concrete value into an interface slice",
			use:       "func Add(rs []Reader) []Reader { return append(rs, Repo{}) }\n",
			dependent: use("Add"),
			provider:  read,
		},
		{
			name:      "append of several variadic values",
			use:       "func Add(rs []Reader) []Reader { return append(rs, Repo{}, &Repo{}) }\n",
			dependent: use("Add"),
			provider:  read,
		},
		{
			name:      "append spreading a slice converts nothing",
			use:       "func Add(rs, more []Reader) []Reader { return append(rs, more...) }\n",
			dependent: use("Add"),
			none:      true,
		},
		{
			name:      "append into an empty interface slice",
			use:       "func Add(rs []any) []any { return append(rs, Repo{}) }\n",
			dependent: use("Add"),
			none:      true,
		},
		{
			name:      "panic takes an empty interface",
			use:       "func Fail() { panic(Repo{}) }\n",
			dependent: use("Fail"),
			none:      true,
		},
		{
			name:      "delete with an interface key",
			use:       "func Drop(m map[Reader]int) { delete(m, Repo{}) }\n",
			dependent: use("Drop"),
			provider:  read,
		},
		{
			name:      "len of a constant array",
			use:       "var Size = len([4]int{})\n",
			dependent: testkit.DeclKey("use.go", entities.KindValue, "Size"),
			none:      true,
		},
	}

	checkFactForms(t, testCases)
}

func TestTypedConversionRuleTuples(t *testing.T) {
	t.Parallel()

	read := testkit.DeclKey("types.go", entities.KindMethod, "Repo.Read")
	pair := "func pair() (Repo, error) { return Repo{}, nil }\n\n"

	testCases := []factForm{
		{
			name:      "a tuple returned into an interface and an error",
			use:       pair + "func New() (Reader, error) { return pair() }\n",
			dependent: useFunc("New"),
			provider:  read,
		},
		{
			name: "a tuple assigned to interface variables",
			use: pair + "func Set() { var r Reader; var err error; r, err = pair(); " +
				"_, _ = r, err }\n",
			dependent: useFunc("Set"),
			provider:  read,
		},
		{
			name: "a tuple declared with an explicit interface type",
			use: "func two() (Repo, Repo) { return Repo{}, Repo{} }\n\n" +
				"func Decl() { var a, b Reader = two(); _, _ = a, b }\n",
			dependent: useFunc("Decl"),
			provider:  read,
		},
		{
			name: "a tuple spread over the parameters of a call",
			use: pair + "func take(Reader, error) {}\n\n" +
				"func Wire() { take(pair()) }\n",
			dependent: useFunc("Wire"),
			provider:  read,
		},
		{
			name: "a comma-ok lookup into an interface variable",
			use: "func Look(m map[string]Repo) { var r Reader; var ok bool; " +
				"r, ok = m[\"k\"]; _, _ = r, ok }\n",
			dependent: useFunc("Look"),
			provider:  read,
		},
		{
			name: "an interface tuple converts nothing",
			use: "func src() (Reader, error) { return nil, nil }\n\n" +
				"func Pass() (Reader, error) { return src() }\n",
			dependent: useFunc("Pass"),
			none:      true,
		},
	}

	checkFactForms(t, testCases)
}

func TestTypedConversionRuleComparisons(t *testing.T) {
	t.Parallel()

	read := testkit.DeclKey("types.go", entities.KindMethod, "Repo.Read")

	testCases := []factForm{
		{
			name:      "a comparison with the concrete value on the right",
			use:       "func Same(r Reader) bool { return r == Repo{} }\n",
			dependent: useFunc("Same"),
			provider:  read,
		},
		{
			name:      "a comparison with the concrete value on the left",
			use:       "func Same(r Reader) bool { return Repo{} == r }\n",
			dependent: useFunc("Same"),
			provider:  read,
		},
		{
			name:      "an inequality",
			use:       "func Differ(r Reader) bool { return r != Repo{} }\n",
			dependent: useFunc("Differ"),
			provider:  read,
		},
		{
			name:      "a switch case of a concrete value on an interface tag",
			use:       "func Pick(r Reader) { switch r {\n\tcase Repo{}:\n\t} }\n",
			dependent: useFunc("Pick"),
			provider:  read,
		},
		{
			name:      "a switch of a concrete tag on an interface case",
			use:       "func Pick(r Reader) { switch (Repo{}) {\n\tcase r:\n\t} }\n",
			dependent: useFunc("Pick"),
			provider:  read,
		},
		{
			name:      "a comparison of two concrete values",
			use:       "func Same(a, b Repo) bool { return a == b }\n",
			dependent: useFunc("Same"),
			none:      true,
		},
		{
			name:      "a comparison with nil",
			use:       "func Unset(r Reader) bool { return r == nil }\n",
			dependent: useFunc("Unset"),
			none:      true,
		},
		{
			name:      "a comparison of an interface with a constant",
			use:       "func Zero(v any) bool { return v == 0 }\n",
			dependent: useFunc("Zero"),
			none:      true,
		},
	}

	checkFactForms(t, testCases)
}

func TestTypedConversionRuleMethodSets(t *testing.T) {
	t.Parallel()

	read := testkit.DeclKey("types.go", entities.KindMethod, "Repo.Read")
	use := useFunc

	testCases := []factForm{
		{
			name: "an instantiated generic interface",
			types: "package fixture\n\ntype Getter[T any] interface{ Get() T }\n\n" +
				"type Box[T any] struct{ v T }\n\nfunc (b Box[T]) Get() T { return b.v }\n",
			use:       "func Wire() Getter[int] { return Box[int]{} }\n",
			dependent: use("Wire"),
			provider:  testkit.DeclKey("types.go", entities.KindMethod, "Box.Get"),
		},
		{
			name:      "an embedded interface",
			types:     conversionTypes + "\ntype Wide interface{ Reader }\n",
			use:       "func Wire() Wide { return Repo{} }\n",
			dependent: use("Wire"),
			provider:  read,
		},
		{
			name: "a pointer receiver reached through an address",
			types: "package fixture\n\ntype Reader interface{ Read() string }\n\n" +
				"type Ptr struct{}\n\nfunc (*Ptr) Read() string { return \"\" }\n",
			use:       "func Wire() Reader { return &Ptr{} }\n",
			dependent: use("Wire"),
			provider:  testkit.DeclKey("types.go", entities.KindMethod, "Ptr.Read"),
		},
		{
			name:      "a method promoted through an embedded struct",
			use:       "type Outer struct{ Repo }\n\nfunc Wire() Reader { return Outer{} }\n",
			dependent: use("Wire"),
			provider:  read,
		},
	}

	checkFactForms(t, testCases)
}

func checkFactForms(t *testing.T, testCases []factForm) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			types := testCase.types
			if types == "" {
				types = conversionTypes
			}

			snap, decls := testkit.Fixture(t, map[string]string{
				"types.go": types,
				"use.go":   "package fixture\n\n" + testCase.use,
			})

			edges, err := Conversion{Source: typed.NewSource(nil)}.Edges(
				context.Background(), snap, decls,
			)
			if err != nil {
				t.Fatalf("Edges() error = %v", err)
			}

			got := testkit.EdgeSet(edges)

			for edge := range got {
				if edge.Provider.Name == "Repo.Write" {
					t.Fatalf("Edges() waits for a method the interface does not list: %+v", edge)
				}

				if testCase.none && edge.Dependent == testCase.dependent {
					t.Fatalf("Edges() = %+v, want no edge from %+v", edge, testCase.dependent)
				}
			}

			want := entities.Edge{
				Dependent: testCase.dependent,
				Provider:  testCase.provider,
				Reason:    entities.ReasonImplements,
			}

			if !testCase.none && !got[want] {
				t.Fatalf("Edges() = %+v, missing %+v", edges, want)
			}
		})
	}
}

func TestTypedComparisonEscapesNothing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		use         string
		wantEscaped bool
	}{
		{"a stored value escapes", "func Keep() any { return Repo{} }\n", true},
		{"a comparison builds a temporary", "func Is(v any) bool { return v == Repo{} }\n", false},
		{"an inequality neither", "func Not(v any) bool { return v != Repo{} }\n", false},
		{"a switch case neither", "func Pick(v any) { switch v {\n\tcase Repo{}:\n\t} }\n", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, map[string]string{
				"types.go": conversionTypes + "\nfunc (Repo) Error() string { return \"\" }\n",
				"use.go":   "package fixture\n\n" + testCase.use,
			})

			facts, err := typed.NewSource(nil).Facts(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			if got := len(facts.Escaped) > 0; got != testCase.wantEscaped {
				t.Fatalf("Escaped = %+v, want escaped %v", facts.Escaped, testCase.wantEscaped)
			}
		})
	}
}

func TestTypedSourceWithoutModuleLoadsNothing(t *testing.T) {
	t.Parallel()

	facts, err := typed.NewSource(nil).Facts(
		context.Background(), entities.Snapshot{Root: t.TempDir()}, nil,
	)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	if len(facts.Forced)+len(facts.Converted) != 0 {
		t.Fatalf("Facts() = %+v, want none", facts)
	}
}
