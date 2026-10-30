package gofacts

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/resolve"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func implementorsFiles() map[string]string {
	return map[string]string{
		"a.go": `package fixture

type Namer interface{ Name() string }

type Setter interface{ Set() }

type Both interface {
	Namer
	Setter
}

type Anything interface{}

type Getter[T any] interface{ Get() T }

type Val struct{}

func (Val) Name() string { return "v" }

type Ptr struct{}

func (*Ptr) Name() string { return "p" }

func (*Ptr) Set() {}

type Box struct{ Val }

type Wrap struct{ Namer }

type Cell[T any] struct{ item T }

func (Cell[T]) Name() string { return "c" }

func (Cell[T]) Get() T { var zero T; return zero }

type IntBox struct{}

func (IntBox) Get() int { return 0 }

type Plain struct{}

type Alias = Namer

func holdLocal() {
	type Local struct{ Namer }

	_ = Local{}
}
`,
		"a_test.go": `package fixture

type InTest struct{}

func (InTest) Name() string { return "t" }
`,
		"x_test.go": `package fixture_test

import "example.test/fixture"

type External struct{ fixture.Val }
`,
	}
}

func TestIndexImplementors(t *testing.T) {
	t.Parallel()

	fix := indexOf(t, implementorsFiles())

	testCases := []struct {
		name  string
		iface string
		want  []string
	}{
		{
			"value receiver, pointer receiver, embedding, extending interfaces and test files",
			"Namer",
			[]string{"Both", "Box", "Cell", "Ptr", "Val", "Wrap", "InTest", "External"},
		},
		{
			"an alias of an interface is asked as the interface is, and is never an answer",
			"Alias",
			[]string{"Both", "Box", "Cell", "Namer", "Ptr", "Val", "Wrap", "InTest", "External"},
		},
		{"an embedded interface needs every method", "Both", []string{"Ptr"}},
		{"an interface implementing an interface", "Setter", []string{"Both", "Ptr"}},
		{"a generic interface unified with its implementors", "Getter", []string{"Cell", "IntBox"}},
		{"an empty interface is excluded", "Anything", nil},
		{"a struct is not an interface", "Val", nil},
		{"an unknown declaration has none", "Missing", nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			want := make([]entities.DeclKey, 0, len(testCase.want))
			for _, name := range testCase.want {
				want = append(want, implementorKey(name))
			}

			got := fix.index.Implementors(typeKey(testCase.iface))
			if !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Fatalf("Implementors(%s) = %v, want %v", testCase.iface, got, want)
			}

			if again := fix.index.Implementors(typeKey(testCase.iface)); !slices.Equal(again, got) {
				t.Fatalf("second Implementors(%s) = %v, want %v", testCase.iface, again, got)
			}
		})
	}
}

func implementorKey(name string) entities.DeclKey {
	switch name {
	case "InTest":
		return testkit.DeclKey("a_test.go", entities.KindType, name)
	case "External":
		return testkit.DeclKey("x_test.go", entities.KindType, name)
	default:
		return typeKey(name)
	}
}

func TestIndexImplementorsAgreeWithResolve(t *testing.T) {
	t.Parallel()

	files := implementorsFiles()
	fix := indexOf(t, files)
	resolver := resolve.NewTyped(fix.source, fix.snap)

	testCases := []string{"Namer", "Setter", "Both", "Anything", "Getter", "Alias"}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			source := files["a.go"]
			offset := strings.Index(source, "type "+name) + len("type ")

			found, err := resolver.Implementations(context.Background(), locationAt(source, offset))
			if err != nil {
				t.Fatalf("Implementations(%s) error = %v", name, err)
			}

			var want []entities.DeclKey

			for _, at := range found {
				if key, isDecl := typeAt(fix.decls, files[at.File], at); isDecl {
					want = append(want, key)
				}
			}

			slices.SortFunc(want, entities.CompareKeys)

			got := fix.index.Implementors(typeKey(name))
			if len(got) != 0 || len(want) != 0 {
				if !slices.Equal(got, want) {
					t.Fatalf("Implementors(%s) = %v, resolve answers %v", name, got, want)
				}
			}
		})
	}
}

func locationAt(source string, offset int) entities.Location {
	start := strings.LastIndex(source[:offset], "\n") + 1

	return entities.Location{
		File: "a.go",
		Line: strings.Count(source[:offset], "\n"),
		Char: offset - start,
	}
}

func typeAt(
	decls []entities.Decl,
	source string,
	at entities.Location,
) (entities.DeclKey, bool) {
	lines := strings.SplitAfter(source, "\n")
	offset := len(strings.Join(lines[:at.Line], "")) + at.Char

	for _, decl := range decls {
		if decl.Key.Kind == entities.KindType && decl.Key.File == at.File &&
			decl.Span.Start <= offset &&
			offset < decl.Span.End {
			return decl.Key, true
		}
	}

	return entities.DeclKey{}, false
}

func BenchmarkImplementorsFirst(b *testing.B) {
	builder, snap, decls := generatedIndex(b)
	iface := testkit.DeclKey("p20/p.go", entities.KindType, "Namer")

	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()

		index, err := New(builder.source).Index(context.Background(), snap, decls)
		if err != nil {
			b.Fatalf("Index() error = %v", err)
		}

		b.StartTimer()

		if got := index.Implementors(iface); len(got) < 400 {
			b.Fatalf("Implementors() = %d types, want every T of every package", len(got))
		}
	}
}

func BenchmarkImplementorsMemoised(b *testing.B) {
	builder, snap, decls := generatedIndex(b)
	iface := testkit.DeclKey("p20/p.go", entities.KindType, "Namer")

	index, err := builder.Index(context.Background(), snap, decls)
	if err != nil {
		b.Fatalf("Index() error = %v", err)
	}

	index.Implementors(iface)
	b.ReportAllocs()

	for b.Loop() {
		if got := index.Implementors(iface); len(got) < 400 {
			b.Fatalf("Implementors() = %d types, want every T of every package", len(got))
		}
	}
}
