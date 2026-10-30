package resolve

import (
	"context"
	"go/types"
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
