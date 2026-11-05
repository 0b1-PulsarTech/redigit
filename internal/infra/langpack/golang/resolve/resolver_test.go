package resolve

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/loadonce"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type spot struct {
	file   string
	needle string
	nth    int
}

type fixture struct {
	files    map[string]string
	source   *typed.Source
	decls    []entities.Decl
	resolver *Typed
}

func newFixture(t *testing.T, files map[string]string) fixture {
	t.Helper()

	snap, decls := testkit.Fixture(t, files)
	source := typed.NewSource(nil)

	return fixture{files: files, source: source, decls: decls, resolver: NewTyped(source, snap)}
}

func (fix fixture) locate(t testing.TB, where spot) entities.Location {
	t.Helper()

	source := fix.files[where.file]
	offset := -1

	for range where.nth + 1 {
		next := strings.Index(source[offset+1:], where.needle)
		if next < 0 {
			t.Fatalf("%s holds no occurrence %d of %q", where.file, where.nth, where.needle)
		}

		offset += next + 1
	}

	line := strings.Count(source[:offset], "\n")
	start := strings.LastIndex(source[:offset], "\n") + 1

	return entities.Location{
		File: where.file,
		Line: line,
		Char: len(utf16.Encode([]rune(source[start:offset]))),
	}
}

func (fix fixture) locations(t testing.TB, spots []spot) []entities.Location {
	t.Helper()

	out := make([]entities.Location, 0, len(spots))
	for _, where := range spots {
		out = append(out, fix.locate(t, where))
	}

	return out
}

func usesFiles() map[string]string {
	return map[string]string{
		"a.go": `package fixture

type Namer interface{ Name() string }

type Base struct{}

func (Base) Hello() string { return "hi" }

type Outer struct{ Base }

type Val struct{}

func (Val) Name() string { return "v" }

func One() int { return 1 }

func Map[T any](items []T, apply func(T) T) []T { return items }

func Run(n Namer, out Outer) string {
	_ = "` + "\U0001F600" + `"; _ = One()
	_ = Map([]int{One()}, func(x int) int { return x })
	_ = Map[int](nil, nil)
	return n.Name() + out.Hello() + Val{}.Name()
}
`,
		"a_test.go":   "package fixture\n\nfunc TestOne() { _ = One() }\n",
		"a_x_test.go": "package fixture_test\n\nimport \"example.test/fixture\"\n\nfunc TestX() { _ = fixture.One() }\n",
		"bad/bad.go":  "package bad\n\nfunc Broken() int { return \"s\" }\n",
	}
}

func TestTypedReferences(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, usesFiles())

	testCases := []struct {
		name  string
		query spot
		want  []spot
	}{
		{
			"a function read from code, a test and an x_test package",
			spot{"a.go", "One() int", 0},
			[]spot{
				{"a.go", "One()", 1},
				{"a.go", "One()", 2},
				{"a_test.go", "One() }", 0},
				{"a_x_test.go", "One()", 0},
			},
		},
		{
			"an interface method is read by the call written against it",
			spot{"a.go", "Name() string }", 0},
			[]spot{{"a.go", "Name()", 2}},
		},
		{
			"a concrete method skips the call written against the interface",
			spot{"a.go", "Name() string { return", 0},
			[]spot{{"a.go", "Name()", 3}},
		},
		{
			"a promoted method is read through the embedding struct",
			spot{"a.go", "Hello() string", 0},
			[]spot{{"a.go", "Hello()", 1}},
		},
		{
			"a generic function is read inferred and instantiated",
			spot{"a.go", "Map[T any]", 0},
			[]spot{{"a.go", "Map([]int", 0}, {"a.go", "Map[int]", 0}},
		},
		{
			"an embedded type is read by the embedding and the receiver",
			spot{"a.go", "Base struct{}", 0},
			[]spot{{"a.go", "Base)", 0}, {"a.go", "Base }", 0}},
		},
		{"a declaration nothing reads", spot{"a.go", "Run(n", 0}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.resolver.References(
				context.Background(), fix.locate(t, testCase.query),
			)
			if err != nil {
				t.Fatalf("References() error = %v", err)
			}

			want := fix.locations(t, testCase.want)
			slices.SortFunc(want, compareLocations)

			if !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Fatalf("References() = %v, want %v", got, want)
			}
		})
	}
}

func TestTypedDefinition(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, usesFiles())

	testCases := []struct {
		name    string
		query   spot
		want    spot
		wantErr error
	}{
		{"a call after an emoji", spot{"a.go", "One()", 1}, spot{"a.go", "One() int", 0}, nil},
		{
			"a use in an x_test package",
			spot{"a_x_test.go", "One()", 0},
			spot{"a.go", "One() int", 0},
			nil,
		},
		{
			"a promoted method call",
			spot{"a.go", "Hello()", 1},
			spot{"a.go", "Hello() string", 0},
			nil,
		},
		{
			"a call through an interface names the interface method",
			spot{"a.go", "Name()", 2},
			spot{"a.go", "Name() string }", 0},
			nil,
		},
		{
			"an instantiated generic call",
			spot{"a.go", "Map[int]", 0},
			spot{"a.go", "Map[T any]", 0},
			nil,
		},
		{"a declaration is not a use", spot{"a.go", "One() int", 0}, spot{}, ErrNotCovered},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.resolver.Definition(context.Background(), fix.locate(t, testCase.query))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Definition() error = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != fix.locate(t, testCase.want) {
				t.Fatalf("Definition() = %v, want %v", got, fix.locate(t, testCase.want))
			}
		})
	}
}

func TestTypedLoadErrorIsNotAMiss(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		kind int
	}{
		{"references", askReferences},
		{"implementations", askImplementations},
		{"definition", askDefinition},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap := entities.Snapshot{Root: t.TempDir(), Files: []string{"go.mod"}}
			secondary := NewMockSecondary(gomock.NewController(t))
			fallback := NewFallback(NewTyped(typed.NewSource(nil), snap), secondary, quietLogger())

			_, err := answerOf(fallback, testCase.kind, entities.Location{File: "a.go"})
			if err == nil || errors.Is(err, ErrNotCovered) {
				t.Fatalf("error = %v, want the load error and not ErrNotCovered", err)
			}

			if fallback.Fallbacks() != 0 {
				t.Fatalf(
					"Fallbacks() = %d, want 0: a load error must not reach the secondary",
					fallback.Fallbacks(),
				)
			}
		})
	}
}

func newBenchFixture(b *testing.B, files map[string]string) fixture {
	b.Helper()

	root := b.TempDir()
	snap := entities.Snapshot{Root: root, Files: []string{"go.mod"}}

	files = maps.Clone(files)
	files["go.mod"] = "module example.test/fixture\n\ngo 1.26\n"

	for name, content := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			b.Fatalf("mkdir for %s: %v", name, err)
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			b.Fatalf("write %s: %v", name, err)
		}

		if name != "go.mod" {
			snap.Files = append(snap.Files, name)
		}
	}

	return fixture{files: files, resolver: NewTyped(typed.NewSource(nil), snap)}
}

func BenchmarkTypedLookup(b *testing.B) {
	fix := newBenchFixture(b, usesFiles())
	ctx := context.Background()
	query := fix.locate(b, spot{"a.go", "One() int", 0})

	started := time.Now()

	if _, err := fix.resolver.References(ctx, query); err != nil {
		b.Fatalf("References() error = %v", err)
	}

	built := time.Since(started)

	b.ResetTimer()

	for range b.N {
		if _, err := fix.resolver.References(ctx, query); err != nil {
			b.Fatalf("References() error = %v", err)
		}
	}

	b.ReportMetric(float64(built.Microseconds())/1000, "build-ms")
}

func TestTypedBuildRefusesAnIndexOfTheMainGroupAlone(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		cancelled bool
		wantErr   error
	}{
		{"an ad hoc pass cut short by a closed load", true, context.Canceled},
		{"an ad hoc pass that finishes", false, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, _ := testkit.Fixture(t, map[string]string{"a.go": "package fixture\n"})
			source := typed.NewSource(nil)

			if _, err := source.Packages(context.Background(), snap); err != nil {
				t.Fatalf("Packages() error = %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if testCase.cancelled {
				cancel()
			}

			_, err := NewTyped(source, snap).build(ctx)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("build() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedBuildSurvivesACancelledFirstCaller(t *testing.T) {
	t.Parallel()

	fix := newFixture(
		t,
		map[string]string{"a.go": "package fixture\n\nfunc One() int { return 1 }\n"},
	)
	where := fix.locate(t, spot{"a.go", "One()", 0})

	gone, cancel := context.WithCancel(context.Background())
	cancel()

	t.Cleanup(func() { _ = fix.resolver.Close(context.Background()) })

	if _, err := fix.resolver.References(gone, where); !errors.Is(err, context.Canceled) {
		t.Fatalf("References() by a cancelled first caller error = %v, want context.Canceled", err)
	}

	if _, err := fix.resolver.References(context.Background(), where); err != nil {
		t.Fatalf("References() by the second caller error = %v", err)
	}
}

func TestTypedClose(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		queried bool
		wantErr error
	}{
		{"before any query nothing is indexed afterwards", false, loadonce.ErrClosed},
		{"after a query the index stays", true, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := newFixture(
				t,
				map[string]string{"a.go": "package fixture\n\nfunc One() int { return 1 }\n"},
			)
			where := fix.locate(t, spot{"a.go", "One()", 0})

			if testCase.queried {
				if _, err := fix.resolver.References(context.Background(), where); err != nil {
					t.Fatalf("References() error = %v", err)
				}
			}

			if err := fix.resolver.Close(context.Background()); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			_, err := fix.resolver.References(context.Background(), where)
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("References() after Close error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
