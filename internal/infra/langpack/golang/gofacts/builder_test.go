package gofacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

const module = "example.test/fixture"

type built struct {
	index  *Index
	snap   entities.Snapshot
	decls  []entities.Decl
	source *typed.Source
}

func indexOf(t *testing.T, files map[string]string) built {
	t.Helper()

	snap, decls := testkit.Fixture(t, files)
	source := typed.NewSource(nil)

	index, err := New(source).Index(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	return built{index: index, snap: snap, decls: decls, source: source}
}

func indexFor(t *testing.T, snap entities.Snapshot, decls []entities.Decl) *Index {
	t.Helper()

	index, err := New(typed.NewSource(nil)).Index(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	return index
}

func cutStatements(
	t *testing.T,
	decls []entities.Decl,
	source string,
	fn entities.DeclKey,
	texts ...string,
) []entities.Decl {
	t.Helper()

	cut := slices.Clone(decls)

	at := slices.IndexFunc(cut, func(decl entities.Decl) bool { return decl.Key == fn })
	if at < 0 {
		t.Fatalf("no declaration %s", fn)
	}

	owner := cut[at]
	from := owner.Span.Start

	for index, text := range texts {
		start := from + strings.Index(source[from:], text)
		if index == 0 {
			cut[at].Span.End = start
		}

		cut = append(cut, entities.Decl{
			Key: entities.DeclKey{
				File: fn.File,
				Kind: entities.KindStmt,
				Name: syntax.StmtName(fn.Name, index),
			},
			Unit: owner.Unit,
			Span: entities.Span{Start: start, End: start + len(text)},
		})
	}

	return cut
}

func funcKey(file, name string) entities.DeclKey {
	return testkit.DeclKey(file, entities.KindFunc, name)
}

func typeKey(name string) entities.DeclKey {
	return testkit.DeclKey("a.go", entities.KindType, name)
}

func methodKey(name string) entities.DeclKey {
	return testkit.DeclKey("a.go", entities.KindMethod, name)
}

func TestBuilderIndexIsSharedPerShape(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		later     bool
		wantSame  bool
		wantCalls int
	}{
		{"the same shape returns the same index", false, true, 0},
		{"a new shape rebuilds from the load already made", true, false, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{"a.go": "package fixture\n\nfunc Plain() {}\n"}
			snap, decls := testkit.Fixture(t, files)
			builder := New(typed.NewSource(nil))

			first, err := builder.Index(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Index() error = %v", err)
			}

			next := decls

			if testCase.later {
				testkit.WriteFile(
					t,
					snap.Root,
					"b.go",
					"package fixture\n\nfunc later() { Plain() }\n",
				)

				next = append(
					slices.Clip(next),
					testkit.DeclareAll(t, snap.Root, []string{"b.go"})...)
			}

			second, err := builder.Index(context.Background(), snap, next)
			if err != nil {
				t.Fatalf("Index() error = %v", err)
			}

			if (first == second) != testCase.wantSame {
				t.Fatalf("same index = %t, want %t", first == second, testCase.wantSame)
			}

			if got := len(second.Calls(funcKey("b.go", "later"))); got != testCase.wantCalls {
				t.Fatalf(
					"calls of a file written after the load = %d, want %d",
					got,
					testCase.wantCalls,
				)
			}
		})
	}
}

func generatedModule(tb testing.TB, packages int) (entities.Snapshot, []entities.Decl) {
	tb.Helper()

	root := tb.TempDir()
	snap := entities.Snapshot{Root: root, Files: []string{"go.mod"}}

	var decls []entities.Decl

	write := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			tb.Fatalf("mkdir %s: %v", rel, err)
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			tb.Fatalf("write %s: %v", rel, err)
		}
	}

	write("go.mod", "module example.test/fixture\n\ngo 1.26\n")

	for pkg := range packages {
		rel := fmt.Sprintf("p%02d/p.go", pkg)
		source := generatedPackage(pkg)
		write(rel, source)

		found, err := syntax.Declarations(context.Background(), rel, []byte(source))
		if err != nil {
			tb.Fatalf("Declarations(%s) error = %v", rel, err)
		}

		snap.Files = append(snap.Files, rel)
		decls = append(decls, found...)
	}

	return snap, decls
}

func generatedPackage(pkg int) string {
	var out strings.Builder

	fmt.Fprintf(&out, "package p%02d\n\nimport \"strings\"\n", pkg)

	if pkg > 0 {
		fmt.Fprintf(&out, "import \"example.test/fixture/p%02d\"\n", pkg-1)
	}

	out.WriteString("\ntype Namer interface{ Name() string }\n")

	for idx := range 10 {
		fmt.Fprintf(
			&out,
			"\ntype T%d struct{}\n\nfunc (T%d) Name() string { return \"t%d\" }\n",
			idx,
			idx,
			idx,
		)
		fmt.Fprintf(
			&out,
			"\nfunc New%d(namer Namer, label string) *T%d { return &T%d{} }\n",
			idx,
			idx,
			idx,
		)
	}

	for idx := range 20 {
		fmt.Fprintf(
			&out,
			"\nfunc F%d(label string) string {\n\tvar one Namer = T%d{}\n",
			idx,
			idx%10,
		)
		fmt.Fprintf(&out, "\tout := one.Name() + strings.ToUpper(label)\n")

		if idx > 0 {
			fmt.Fprintf(&out, "\tout += F%d(\"k\")\n", idx-1)
		} else if pkg > 0 {
			fmt.Fprintf(&out, "\tout += p%02d.F0(\"seed\")\n", pkg-1)
		}

		out.WriteString("\treturn out\n}\n")
	}

	return out.String()
}

func generatedIndex(tb testing.TB) (*Builder, entities.Snapshot, []entities.Decl) {
	tb.Helper()

	snap, decls := generatedModule(tb, 40)
	builder := New(typed.NewSource(nil))

	if _, err := builder.Index(context.Background(), snap, decls); err != nil {
		tb.Fatalf("Index() error = %v", err)
	}

	return builder, snap, decls
}

func BenchmarkBuilderIndex(b *testing.B) {
	snap, decls := generatedModule(b, 40)
	source := typed.NewSource(nil)

	if _, err := source.Packages(context.Background(), snap); err != nil {
		b.Fatalf("Packages() error = %v", err)
	}

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	var last *Index

	b.ReportAllocs()

	for b.Loop() {
		index, err := New(source).Index(context.Background(), snap, decls)
		if err != nil {
			b.Fatalf("Index() error = %v", err)
		}

		last = index
	}

	runtime.GC()
	runtime.ReadMemStats(&after)

	calls := 0
	for _, found := range last.calls {
		calls += len(found)
	}

	b.ReportMetric(float64(len(decls)), "decls")
	b.ReportMetric(float64(calls), "calls")
	b.ReportMetric(float64(len(last.ctors)), "ctors")
	b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/(1<<20), "heap-MB")
	runtime.KeepAlive(last)
}

func TestBuilderIndexLoadFailure(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{"a.go": "package fixture\n"})
	source := typed.NewSource(nil)

	if err := source.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := New(source).Index(context.Background(), snap, decls); err == nil {
		t.Fatal("Index() after Close error = nil, want the load failure")
	}
}

func TestIndexNamesAnExportDataDeclByItsKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		dir  string
	}{
		{name: "a package under testdata", dir: "testdata/lib"},
		{name: "a package under a dir the go tool skips", dir: "_dir/lib"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			file := testCase.dir + "/lib.go"
			fix := indexOf(t, map[string]string{
				file: "package lib\n\nfunc Big() {\n" + strings.Repeat("\n", 15) + "}\n\n" +
					"func Small() {}\n\ntype Value struct{}\n\nfunc (Value) Method() {}\n",
				"use/use.go": "package use\n\nimport \"" + module + "/" + testCase.dir +
					"\"\n\nfunc Use() {\n\tlib.Small()\n\tlib.Value{}.Method()\n}\n",
			})

			var got []entities.DeclKey

			for _, call := range fix.index.Calls(funcKey("use/use.go", "Use")) {
				got = append(got, call.CalleeKey, call.Receiver.Key)
			}

			want := []entities.DeclKey{
				funcKey(file, "Small"),
				{},
				testkit.DeclKey(file, entities.KindMethod, "Value.Method"),
				testkit.DeclKey(file, entities.KindType, "Value"),
			}

			if !slices.Equal(got, want) {
				t.Fatalf("callee and receiver keys = %v, want %v", got, want)
			}
		})
	}
}

func TestIndexKeysACalleeOfAParsedFileByItsPosition(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		repeats int
	}{
		{name: "a tree that lists each decl once", repeats: 1},
		{name: "a tree that lists each decl twice", repeats: 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := indexOf(t, map[string]string{
				"a.go": "package fixture\n\nfunc Plain() {}\n\nfunc Use() {\n\tPlain()\n}\n",
			})

			listed := slices.Repeat(fix.decls, testCase.repeats)
			calls := indexFor(t, fix.snap, listed).Calls(funcKey("a.go", "Use"))

			if len(calls) != 1 || calls[0].CalleeKey != funcKey("a.go", "Plain") {
				t.Fatalf("Calls(Use) = %+v, want one call keyed to Plain", calls)
			}
		})
	}
}
