package gofacts

import (
	"context"
	"errors"
	"path"
	"reflect"
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func wireIndex(t *testing.T) *Index {
	t.Helper()

	source := "package fixture\n\nfunc Plain() {}\n\nfunc take(string) {}\n\n" +
		"type Repo struct{}\n\nfunc (Repo) Get(string) {}\n\n" +
		"func wire() {\n\tPlain()\n\ttake(\"s\")\n\tRepo{}.Get(\"z\")\n}\n"
	snap, decls := testkit.Fixture(t, map[string]string{"a.go": source})
	cut := cutStatements(
		t, decls, source, funcKey("a.go", "wire"), "Plain()", "take(\"s\")", "Repo{}.Get(\"z\")",
	)

	return indexFor(t, snap, cut)
}

func stmtKey(position int) entities.DeclKey {
	return testkit.DeclKey("a.go", entities.KindStmt, "wire#"+strconv.Itoa(position))
}

func TestIndexStmtCallsInto(t *testing.T) {
	t.Parallel()

	index := wireIndex(t)

	testCases := []struct {
		name    string
		stmt    entities.DeclKey
		pattern string
		want    []facts.Call
	}{
		{
			"a short function name",
			stmtKey(0),
			"Plain",
			[]facts.Call{{
				Site: stmtKey(0), Callee: module + ".Plain", CalleeKey: funcKey("a.go", "Plain"),
			}},
		},
		{
			"a short method name",
			stmtKey(2),
			"Repo.*",
			[]facts.Call{{
				Site: stmtKey(2), Callee: module + ".Repo.Get", CalleeKey: methodKey("Repo.Get"),
				Receiver: facts.TypeRef{Key: typeKey("Repo"), Name: module + ".Repo"},
				Literals: []string{"z"},
			}},
		},
		{
			"a qualified name",
			stmtKey(1),
			module + ".take",
			[]facts.Call{{
				Site: stmtKey(1), Callee: module + ".take",
				CalleeKey: funcKey("a.go", "take"), Literals: []string{"s"},
			}},
		},
		{"no match", stmtKey(0), "Open", nil},
		{"a statement with no calls recorded", stmtKey(7), "*", nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := index.StmtCallsInto(testCase.stmt, testCase.pattern)
			if err != nil {
				t.Fatalf("StmtCallsInto(%q) error = %v", testCase.pattern, err)
			}

			if len(got) == 0 && len(testCase.want) == 0 {
				return
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("StmtCallsInto(%q) = %+v, want %+v", testCase.pattern, got, testCase.want)
			}
		})
	}
}

func TestIndexStmtCallsIntoRefusesABadPattern(t *testing.T) {
	t.Parallel()

	index := wireIndex(t)

	testCases := []struct {
		name string
		stmt entities.DeclKey
	}{
		{"a statement with calls", stmtKey(0)},
		{"a statement with none", stmtKey(7)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := index.StmtCallsInto(testCase.stmt, "[")
			if !errors.Is(err, path.ErrBadPattern) || got != nil {
				t.Fatalf("StmtCallsInto() = %v, %v, want path.ErrBadPattern", got, err)
			}
		})
	}
}

func BenchmarkIndexCalls(b *testing.B) {
	builder, snap, decls := generatedIndex(b)

	index, err := builder.Index(context.Background(), snap, decls)
	if err != nil {
		b.Fatalf("Index() error = %v", err)
	}

	site := testkit.DeclKey("p20/p.go", entities.KindFunc, "F5")

	b.ReportAllocs()

	for b.Loop() {
		if len(index.Calls(site)) == 0 {
			b.Fatal("Calls() found none")
		}
	}
}

func BenchmarkIndexCtorArgTypes(b *testing.B) {
	builder, snap, decls := generatedIndex(b)

	index, err := builder.Index(context.Background(), snap, decls)
	if err != nil {
		b.Fatalf("Index() error = %v", err)
	}

	fn := testkit.DeclKey("p20/p.go", entities.KindFunc, "New3")

	b.ReportAllocs()

	for b.Loop() {
		if taken, takeErr := index.CtorArgTypes(fn); takeErr != nil || len(taken) != 2 {
			b.Fatalf("CtorArgTypes() = %v, %v", taken, takeErr)
		}
	}
}
