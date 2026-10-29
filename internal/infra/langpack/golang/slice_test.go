package golang

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

const sliceSource = `package app

import (
	"fmt"
	"os"
)

func Mount(srv *Server) error {
	cfg, err := load()
	if err != nil {
		return err
	}

	// the api routes
	srv.Add(cfg)
	srv.Add(other())

	return nil
}

func Register(srv *Server) {
	srv.Add(one())
	srv.Add(two())
}

func Early(srv *Server) error {
	srv.Add(one())
	return nil
	srv.Add(two())
}

func Jump() {
again:
	run()
	goto again
}

func Single() {
	run()
}

func Table() []step {
	return []step{
		{name: "a", run: runA},
		{name: "b", run: runB},
	}
}

func Chain() []step {
	return slices.Concat(tableA(), tableB())
}

func Use() {
	fmt.Println(os.Args)
	println("x")
}
`

func sliceOf(t *testing.T, name string) (entities.Decl, bool, []entities.Decl, []entities.Edge) {
	t.Helper()

	syntax := NewSyntax()
	source := []byte(sliceSource)

	decls, err := syntax.Declarations(context.Background(), "app/app.go", source)
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	for _, decl := range decls {
		if decl.Key.Name != name {
			continue
		}

		sliced, ok, err := syntax.Slice(context.Background(), decl, source)
		if err != nil {
			t.Fatalf("Slice(%s) error = %v", name, err)
		}

		return sliced.Head, ok, sliced.Members, sliced.Edges
	}

	t.Fatalf("no declaration %q", name)

	return entities.Decl{}, false, nil, nil
}

func TestSliceRefusesWhatCannotLoseStatements(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		decl string
	}{
		{"an early top-level return makes the rest unreachable", "Early"},
		{"a label ties statements by position", "Jump"},
		{"a single statement has nothing to cut", "Single"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, ok, _, _ := sliceOf(t, testCase.decl); ok {
				t.Fatalf("Slice(%s) cut the function, want it left whole", testCase.decl)
			}
		})
	}
}

func TestSliceBindsTheSignatureAndTheLocals(t *testing.T) {
	t.Parallel()

	head, ok, members, edges := sliceOf(t, "Mount")
	if !ok {
		t.Fatal("Slice(Mount) refused a cuttable body")
	}

	if len(members) != 5 {
		t.Fatalf("members = %d, want the five top-level statements", len(members))
	}

	has := func(from, to int) bool {
		return slices.ContainsFunc(edges, func(edge entities.Edge) bool {
			return edge.Dependent.Name == syntax.StmtName("Mount", from) &&
				edge.Provider.Name == syntax.StmtName("Mount", to) &&
				edge.Reason == entities.ReasonBody
		})
	}

	// `err` is read by the check, `cfg` by the first route.
	if !has(1, 0) || !has(2, 0) {
		t.Fatalf("edges = %v, want the check and the route to need the declaration", edges)
	}

	if has(3, 0) {
		t.Fatalf("edges = %v, a route that reads no local must not wait for it", edges)
	}

	headEdge := slices.ContainsFunc(edges, func(edge entities.Edge) bool {
		return edge.Dependent.Name == syntax.StmtName("Mount", 3) && edge.Provider == head.Key
	})
	if !headEdge {
		t.Fatalf("edges = %v, want every statement to need the signature", edges)
	}
}

func TestSliceBirthsTheDeclarationWithItsFirstReader(t *testing.T) {
	t.Parallel()

	head, _, members, _ := sliceOf(t, "Mount")

	// A function with results is born with its last statement, so it compiles;
	// `cfg` is declared and must be read in the same commit.
	if head.Atom == "" || head.Atom != members[4].Atom {
		t.Fatalf(
			"head atom = %q, last = %q, want the return born with the head",
			head.Atom,
			members[4].Atom,
		)
	}

	if members[0].Atom == "" || members[0].Atom != members[1].Atom {
		t.Fatalf("atoms = %q and %q, want the declaration born with its first reader",
			members[0].Atom, members[1].Atom)
	}
}

func TestSliceBirthsAFunctionWithoutResultsWithItsFirstStatement(t *testing.T) {
	t.Parallel()

	head, _, members, _ := sliceOf(t, "Register")

	// A signature born alone would render as the whole function.
	if head.Atom == "" || head.Atom != members[0].Atom || members[1].Atom != "" {
		t.Fatalf("atoms head=%q first=%q second=%q, want only the head and the first together",
			head.Atom, members[0].Atom, members[1].Atom)
	}
}

func TestRenderWritesOnlyTheKeptStatements(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax()
	source := []byte(sliceSource)
	head, _, members, _ := sliceOf(t, "Register")

	testCases := []struct {
		name string
		keep []entities.DeclKey
		has  []string
		not  []string
	}{
		{
			"the head and the first statement",
			[]entities.DeclKey{head.Key, members[0].Key},
			[]string{"one()"},
			[]string{"two()"},
		},
		{
			"both statements",
			[]entities.DeclKey{head.Key, members[0].Key, members[1].Key},
			[]string{"one()", "two()"},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := syntax.Render(context.Background(), source, testCase.keep)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			for _, want := range testCase.has {
				if !strings.Contains(string(got), want) {
					t.Fatalf("Render() = %s, want %q", got, want)
				}
			}

			for _, unwanted := range testCase.not {
				if strings.Contains(string(got), unwanted) {
					t.Fatalf("Render() = %s, want no %q", got, unwanted)
				}
			}
		})
	}
}

func TestRenderWritesAFunctionNoStatementNamesWhole(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax()
	key := entities.DeclKey{File: "app/app.go", Kind: entities.KindFunc, Name: "Use"}

	got, err := syntax.Render(context.Background(), []byte(sliceSource), []entities.DeclKey{key})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(string(got), `println("x")`) || !strings.Contains(string(got), "os.Args") {
		t.Fatalf("Render() = %s, want the whole body", got)
	}
}

// A table of registrations grows one element per feature: each element is a
// piece, and the function is born with the first.
func TestSliceCutsAReturnedListByElement(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax()
	source := []byte(sliceSource)

	for _, name := range []string{"Table", "Chain"} {
		head, ok, members, _ := sliceOf(t, name)
		if !ok || len(members) != 2 {
			t.Fatalf("Slice(%s) = %v with %d members, want 2 elements", name, ok, len(members))
		}

		if head.Atom == "" || head.Atom != members[0].Atom || members[1].Atom != "" {
			t.Fatalf("atoms head %q first %q second %q, want head born with the first element",
				head.Atom, members[0].Atom, members[1].Atom)
		}

		got, err := syntax.Render(
			context.Background(), source, []entities.DeclKey{head.Key, members[1].Key},
		)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}

		if strings.Contains(string(got), "runA") || strings.Contains(string(got), "tableA") {
			t.Fatalf("Render(%s) = %s, want only the kept element", name, got)
		}
	}
}
