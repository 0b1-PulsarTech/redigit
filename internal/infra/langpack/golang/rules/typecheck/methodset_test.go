package typecheck

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

const methodSetBase = `package fixture

type errorInterface[Self any] interface {
	*Self
	error
}

type BaseError[Self any, SelfPtr errorInterface[Self]] struct{ Err error }

func (b BaseError[Self, SelfPtr]) Unwrap() error { return b.Err }
`

func TestMethodSetRule(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		files     map[string]string
		wantAtom  []entities.DeclKey
		wantEdges []entities.Edge
	}{
		{
			name: "the type that instantiates its own constraint is one atom with the method it forces",
			files: map[string]string{
				"base.go": methodSetBase,
				"missing.go": `package fixture

type ErrMissing struct {
	BaseError[ErrMissing, *ErrMissing]
	Name string
}
`,
				"missing_methods.go": `package fixture

func (e *ErrMissing) Error() string { return "missing " + e.Name }

func (e *ErrMissing) Status() int { return 404 }
`,
			},
			wantAtom: []entities.DeclKey{
				testkit.DeclKey("missing.go", entities.KindType, "ErrMissing"),
				testkit.DeclKey("missing_methods.go", entities.KindMethod, "ErrMissing.Error"),
			},
		},
		{
			name: "another declaration instantiating it only waits for the method",
			files: map[string]string{
				"base.go": methodSetBase,
				"missing.go": `package fixture

type ErrMissing struct{ Name string }

func (e *ErrMissing) Error() string { return e.Name }
`,
				"use.go": `package fixture

var Wrapped BaseError[ErrMissing, *ErrMissing]
`,
			},
			wantEdges: []entities.Edge{{
				Dependent: testkit.DeclKey("use.go", entities.KindValue, "Wrapped"),
				Provider:  testkit.DeclKey("missing.go", entities.KindMethod, "ErrMissing.Error"),
				Reason:    entities.ReasonMethodSet,
			}},
		},
		{
			name: "a constraint that names no method forces none",
			files: map[string]string{
				"keys.go": `package fixture

type Set[K comparable] struct{ items map[K]struct{} }

type Names struct{ Set[string] }
`,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, testCase.files)
			rule := MethodSet{Source: typed.NewSource(nil)}

			checkAtoms(t, rule, snap, decls, testCase.wantAtom)
			checkEdges(t, rule, snap, decls, testCase.wantEdges)
		})
	}
}

func checkAtoms(
	t *testing.T,
	rule MethodSet,
	snap entities.Snapshot,
	decls []entities.Decl,
	want []entities.DeclKey,
) {
	t.Helper()

	atoms, err := rule.Atoms(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Atoms() error = %v", err)
	}

	if len(atoms) != len(want) {
		t.Fatalf("Atoms() = %v, want members %v", atoms, want)
	}

	for _, member := range want {
		if atoms[member] == "" || atoms[member] != atoms[want[0]] {
			t.Fatalf("Atoms() = %v, want %v in one atom", atoms, want)
		}
	}
}

func checkEdges(
	t *testing.T,
	rule MethodSet,
	snap entities.Snapshot,
	decls []entities.Decl,
	want []entities.Edge,
) {
	t.Helper()

	edges, err := rule.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	got := testkit.EdgeSet(edges)
	if len(got) != len(want) {
		t.Fatalf("Edges() = %+v, want %+v", edges, want)
	}

	for _, edge := range want {
		if !got[edge] {
			t.Fatalf("Edges() = %+v, missing %+v", edges, edge)
		}
	}
}
