//go:build integration

package lsp

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// This is the proof the PLANO.md blind spot is closed: a file behind
// //go:build integration is invisible to textDocument/references unless
// BuildFlags carries the matching -tags flag, silently and without error.
// It drives a real gopls process against a throwaway module, never a fake.
func TestSession_References_RequiresBuildTag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixtureModule(t, dir)

	// "func Target() {}" on the third line (0-based line 2); "Target"
	// starts right after "func ".
	at := entities.Location{File: filepath.Join(dir, "normal.go"), Line: 2, Char: 5}

	testCases := []struct {
		name       string
		buildFlags []string
		wantTagged bool
	}{
		{
			name:       "with build tag sees the tagged call site",
			buildFlags: []string{"-tags=integration"},
			wantTagged: true,
		},
		{name: "without build tag misses the tagged call site", buildFlags: nil, wantTagged: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Each subtest gets its own context: sharing one built in the
			// parent with a deferred cancel would cancel it the instant the
			// parent's body returns, which happens before parallel subtests
			// actually run their bodies.
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			session, err := StartSession(ctx, ServerConfig{
				Command:    "gopls",
				Args:       []string{"-mode=stdio"},
				RootPath:   dir,
				BuildFlags: testCase.buildFlags,
			})
			if err != nil {
				t.Fatalf("StartSession: %v", err)
			}

			defer func() {
				if closeErr := session.Close(ctx); closeErr != nil {
					t.Logf("Close: %v", closeErr)
				}
			}()

			refs, err := session.References(ctx, at)
			if err != nil {
				t.Fatalf("References: %v", err)
			}

			gotTagged := slices.ContainsFunc(refs, func(loc entities.Location) bool {
				return filepath.Base(loc.File) == "tagged.go"
			})
			if gotTagged != testCase.wantTagged {
				t.Fatalf(
					"references saw tagged.go = %v, want %v (refs=%+v)",
					gotTagged,
					testCase.wantTagged,
					refs,
				)
			}
		})
	}
}

// writeFixtureModule creates a minimal Go module under dir with a plain
// declaration and a build-tagged caller of it, the exact shape the hard
// facts describe: a normal file's declaration, referenced only from a file
// behind //go:build integration.
func writeFixtureModule(t testing.TB, dir string) {
	t.Helper()

	files := map[string]string{
		"go.mod":    "module example.com/fixture\n\ngo 1.26\n",
		"normal.go": "package fixture\n\nfunc Target() {}\n",
		"tagged.go": "//go:build integration\n\npackage fixture\n\nfunc CallSite() {\n\tTarget()\n}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func writeModule(t testing.TB, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func matchModule() map[string]string {
	files := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.26\n",
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
		"generic.go": `package fixture

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
`,
	}

	maps.Copy(files, clauseModule())
	maps.Copy(files, skippedModule())

	return files
}

func clauseModule() map[string]string {
	return map[string]string{
		"testdata/a/a.go": "package a\n\ntype Speaker interface{ Speak() }\n\ntype Near struct{}\n\n" +
			"func (Near) Speak() {}\n",
		"testdata/b/b.go":  "package b\n\ntype Far struct{}\n\nfunc (Far) Speak() {}\n",
		"unify.go":         unifySource,
		"alias.go":         aliasSource,
		"sub/mark.go":      "package sub\n\nfunc Mark() {}\n",
		"pk/pk.go":         "package pk\n",
		"pk/in_test.go":    "package pk\n",
		"pk/x_test.go":     "package pk_test\n\nimport _ \"example.com/fixture/pk\"\n",
		"pkuser/user.go":   "package pkuser\n\nimport _ \"example.com/fixture/pk\"\n",
		"pkuser/x_test.go": "package pkuser_test\n\nimport _ \"example.com/fixture/pk\"\n",
		"bad/bad.go": "package bad\n\nimport \"example.com/fixture/sub\"\n\n" +
			"func Broken() int { sub.Mark(); return \"s\" }\n",
		"embed.go": "package fixture\n\ntype Namer interface{ String() string }\n\n" +
			"type Stringy[T any] interface{ Take(interface{ Namer; Label() T }) }\n\n" +
			"type stringyInt struct{}\n\n" +
			"func (stringyInt) Take(interface{ Namer; Label() int }) {}\n",
		"rc/rc.go":      "package rc\n\ntype T struct{}\n",
		"rc/in_test.go": "package rc\n",
		"rc/x_test.go": "package rc_test\n\nimport (\n\t\"example.com/fixture/rc\"\n" +
			"\t\"example.com/fixture/rchelp\"\n)\n\nfunc use() { rchelp.F() }\n\n" +
			"type Dot struct{}\n\nfunc (Dot) Draw(rc.T) {}\n",
		"rchelp/help.go": "package rchelp\n\nimport \"example.com/fixture/rc\"\n\n" +
			"func F() { _ = rc.T{} }\n\ntype Shape interface{ Draw(rc.T) }\n",
		"doc.go":        "package fixture\n",
		"ta/ta.go":      "package ta\n\ntype X struct{}\n",
		"ta/ta_test.go": "package ta\n\ntype Getter interface{ Get() X }\n",
		"ta/x_test.go":  "package ta_test\n\nimport _ \"example.com/fixture/tas\"\n",
		"tar/r.go": "package tar\n\nimport \"example.com/fixture/ta\"\n\ntype T struct{}\n\n" +
			"func (T) Get() ta.X { return ta.X{} }\n",
		"tas/s.go": "package tas\n\nimport \"example.com/fixture/ta\"\n\ntype U struct{}\n\n" +
			"func (U) Get() ta.X { return ta.X{} }\n",
		"tan/n.go": "package tan\n\ntype X struct{}\n\ntype N struct{}\n\n" +
			"func (N) Get() X { return X{} }\n",
		"tb/tb.go": "package tb\n\ntype X struct{}\n",
		"tb/tb_test.go": "package tb\n\ntype fake struct{}\n\n" +
			"func (fake) Get() X { return X{} }\n",
		"tbp/port.go": "package tbp\n\nimport \"example.com/fixture/tb\"\n\n" +
			"type Port interface{ Get() tb.X }\n",
		"sub/user.go": `package sub

import "example.com/fixture"

func F() {
	type local struct{ fixture.Plain }
	_ = local{}
}
`,
		"sub/named.go": "package sub\n\nimport fx \"example.com/fixture\"\n\nvar _ fx.Plain\n",
		"sub/blank.go": "package sub\n\nimport _ \"example.com/fixture\"\n",
	}
}

type matchCase struct {
	name       string
	references bool
	at         entities.Location
	want       []entities.Location
}

func matchCases(dir string) []matchCase {
	return []matchCase{
		{
			"a generic type implements by its type parameters standing for any type, " +
				"a local type of the same package does too, one of another package does not",
			false,
			entities.Location{File: filepath.Join(dir, "cell.go"), Line: 6, Char: 5},
			[]entities.Location{
				{File: "cell.go", Line: 2, Char: 5},
				{File: "gen.go", Line: 12, Char: 20},
				{File: "plain.go", Line: 2, Char: 5},
			},
		},
		{
			"a type parameter in a parameter position",
			false,
			entities.Location{File: filepath.Join(dir, "cell.go"), Line: 8, Char: 5},
			[]entities.Location{{File: "gen.go", Line: 2, Char: 5}},
		},
		{
			"a generic interface is implemented by the type fixing its parameter",
			false,
			entities.Location{File: filepath.Join(dir, "generic.go"), Line: 4, Char: 5},
			[]entities.Location{{File: "generic.go", Line: 6, Char: 5}},
		},
		{
			"an instantiated named type argument",
			false,
			entities.Location{File: filepath.Join(dir, "generic.go"), Line: 16, Char: 5},
			[]entities.Location{{File: "generic.go", Line: 18, Char: 5}},
		},
		{
			"a channel element",
			false,
			entities.Location{File: filepath.Join(dir, "generic.go"), Line: 22, Char: 5},
			[]entities.Location{{File: "generic.go", Line: 24, Char: 5}},
		},
		{
			"an array element",
			false,
			entities.Location{File: filepath.Join(dir, "generic.go"), Line: 28, Char: 5},
			[]entities.Location{{File: "generic.go", Line: 30, Char: 5}},
		},
		{
			"one type parameter bound to two different types",
			false,
			entities.Location{File: filepath.Join(dir, "generic.go"), Line: 34, Char: 5},
			nil,
		},
		{
			"one candidate type parameter bound to two different types",
			false,
			entities.Location{File: filepath.Join(dir, "generic.go"), Line: 40, Char: 5},
			nil,
		},
		{
			"a type of a testdata directory once asked about, so its sibling can see it",
			false,
			entities.Location{File: filepath.Join(dir, "testdata/b/b.go"), Line: 2, Char: 5},
			nil,
		},
		{
			"an interface of one testdata directory against a type of its sibling",
			false,
			entities.Location{File: filepath.Join(dir, "testdata/a/a.go"), Line: 2, Char: 5},
			[]entities.Location{
				{File: "testdata/a/a.go", Line: 4, Char: 5},
				{File: "testdata/b/b.go", Line: 2, Char: 5},
			},
		},
		{
			"the package clause of a file with no declaration is read by the import specs",
			true,
			entities.Location{File: filepath.Join(dir, "doc.go"), Line: 0, Char: 9},
			[]entities.Location{
				{File: "sub/blank.go", Line: 2, Char: 7},
				{File: "sub/named.go", Line: 2, Char: 7},
				{File: "sub/user.go", Line: 2, Char: 7},
			},
		},
	}
}

func TestSession_AnswersTheTypedResolverMustMatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeModule(t, dir, matchModule())

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	session, err := StartSession(ctx, ServerConfig{
		Command: "gopls", Args: []string{"-mode=stdio"}, RootPath: dir,
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	defer func() { _ = session.Close(ctx) }()

	allCases := slices.Concat(matchCases(dir), unifyCases(dir), aliasCases(dir), skippedCases(dir))

	for _, testCase := range allCases {
		query := session.Implementations
		if testCase.references {
			query = session.References
		}

		got, err := query(ctx, testCase.at)
		if err != nil {
			t.Errorf("%s: %v", testCase.name, err)

			continue
		}

		slices.SortFunc(got, func(left, right entities.Location) int {
			return strings.Compare(
				fmt.Sprintf("%s:%03d:%03d", left.File, left.Line, left.Char),
				fmt.Sprintf("%s:%03d:%03d", right.File, right.Line, right.Char),
			)
		})

		if !slices.Equal(got, testCase.want) {
			t.Errorf("%s: got %+v, want %+v", testCase.name, got, testCase.want)
		}
	}
}
