package resolve

import (
	"context"
	"go/types"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

func TestSatisfies(t *testing.T) {
	t.Parallel()

	snap, _ := testkit.Fixture(t, implementsFiles())

	roots, err := typed.NewSource(nil).Packages(context.Background(), snap)
	if err != nil || len(roots) == 0 {
		t.Fatalf("Packages() = %d roots, error = %v", len(roots), err)
	}

	scope := roots[0].Types.Scope()

	testCases := []struct {
		name      string
		candidate string
		iface     string
		want      bool
	}{
		{"a value receiver", "Val", "Namer", true},
		{"a method the type lacks", "Val", "Setter", false},
		{"a pointer receiver counts through the pointer", "Ptr", "Both", true},
		{"a method promoted by embedding", "Box", "Namer", true},
		{"a generic type unified with the method", "Cell", "Namer", true},
		{"a type with no method", "Plain", "Namer", false},
		{"an interface that extends the target", "Both", "Namer", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			named, isNamed := scope.Lookup(testCase.candidate).Type().(*types.Named)
			target, isIface := scope.Lookup(testCase.iface).Type().Underlying().(*types.Interface)

			if !isNamed || !isIface {
				t.Fatalf("%s or %s is not declared as expected", testCase.candidate, testCase.iface)
			}

			if got := Satisfies(named, roots[0].Fset, target); got != testCase.want {
				t.Fatalf("Satisfies(%s, %s) = %t, want %t",
					testCase.candidate, testCase.iface, got, testCase.want)
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
