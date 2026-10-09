package config

import (
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// The first package importing a dependency was born hundreds of commits before
// the require that provides it: "no required module provides package".
func TestImportRuleMakesAFileWaitForTheRequireProvidingItsImports(t *testing.T) {
	t.Parallel()

	const (
		appMod = "apps/lead/go.mod"
		appGo  = "apps/lead/x.go"
	)

	requireOf := func(module string) entities.Decl {
		return piece(appMod, "require "+module, entities.Span{})
	}

	testCases := []struct {
		name    string
		source  string
		extra   []entities.Decl
		want    []string
		wantNot []string
	}{
		{
			name:   "an external package waits for its module",
			source: "package lead\n\nimport \"entgo.io/ent/dialect/sql\"\n",
			extra:  []entities.Decl{requireOf("entgo.io/ent")},
			want:   []string{"entgo.io/ent"},
		},
		{
			name:   "the longest required prefix owns the import",
			source: "package lead\n\nimport \"example.com/a/b/pkg\"\n",
			extra:  []entities.Decl{requireOf("example.com/a"), requireOf("example.com/a/b")},
			want:   []string{"example.com/a/b"},
			wantNot: []string{
				"example.com/a",
			},
		},
		{
			name:    "a prefix that is not a path boundary does not match",
			source:  "package lead\n\nimport \"example.com/ab/pkg\"\n",
			extra:   []entities.Decl{requireOf("example.com/a")},
			wantNot: []string{"example.com/a"},
		},
		{
			name:    "the standard library waits for nothing",
			source:  "package lead\n\nimport \"context\"\n",
			extra:   []entities.Decl{requireOf("example.com/a")},
			wantNot: []string{"example.com/a"},
		},
		{
			name:    "a file that does not parse yields no edge",
			source:  "this is not go",
			extra:   []entities.Decl{requireOf("example.com/a")},
			wantNot: []string{"example.com/a"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap := treeWith(t, map[string]string{appGo: testCase.source, appMod: "module m\n"})
			header := piece(appMod, "github.com/org/lead", entities.Span{})
			decl := goDecl(appGo, "Handler")
			decls := append([]entities.Decl{header, decl}, testCase.extra...)

			edges, err := (importRule{}).Edges(t.Context(), snap, decls)
			if err != nil {
				t.Fatalf("Edges() = %v", err)
			}

			assertRequireEdges(t, edges, decl.Key, testCase.want, testCase.wantNot)
		})
	}
}

func assertRequireEdges(
	t *testing.T,
	edges []entities.Edge,
	dependent entities.DeclKey,
	want, wantNot []string,
) {
	t.Helper()

	requireKey := func(module string) entities.DeclKey {
		return entities.DeclKey{
			File: "apps/lead/go.mod", Kind: entities.KindOther, Name: "require " + module,
		}
	}

	for _, module := range want {
		if !edgeBetween(edges, dependent, requireKey(module)) {
			t.Errorf("the file does not wait for require %s; edges: %v", module, edges)
		}
	}

	for _, module := range wantNot {
		if edgeBetween(edges, dependent, requireKey(module)) {
			t.Errorf("the file waits for require %s it does not import", module)
		}
	}
}

// A file belongs to the NEAREST go.mod: the outer module does not compile the
// inner one's packages, so its requires say nothing about them.
func TestImportRuleReadsTheNearestModulesRequires(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"go.mod":         "module outer\n",
		"inner/go.mod":   "module inner\n",
		"inner/pkg/x.go": "package pkg\n\nimport \"example.com/a\"\n",
	})

	outerHeader := piece("go.mod", "outer", entities.Span{})
	innerHeader := piece("inner/go.mod", "inner", entities.Span{})
	outerRequire := piece("go.mod", "require example.com/a", entities.Span{})
	innerRequire := piece("inner/go.mod", "require example.com/a", entities.Span{})
	decl := goDecl("inner/pkg/x.go", "F")

	edges, err := (importRule{}).Edges(
		t.Context(), snap,
		[]entities.Decl{outerHeader, innerHeader, outerRequire, innerRequire, decl},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, decl.Key, innerRequire.Key) {
		t.Error("the file does not wait for the inner module's require")
	}

	if edgeBetween(edges, decl.Key, outerRequire.Key) {
		t.Error("the file waits for the outer module's require")
	}
}

// Go prunes the module graph: a module providing a package the build imports
// only through a dependency must be listed in the main go.mod, as an indirect
// require, or the go command answers "updates to go.mod needed".
func TestImportRuleMakesADirectRequireWaitForTheIndirectOnes(t *testing.T) {
	t.Parallel()

	const modFile = "apps/lead/go.mod"

	body := "module m\n\ngo 1.26\n\nrequire example.com/direct v1.0.0\n\n" +
		"require example.com/transitive v1.0.0 // indirect\n"
	boundary := len("module m\n\ngo 1.26\n\nrequire example.com/direct v1.0.0\n\n")

	snap := treeWith(t, map[string]string{modFile: body})

	header := piece(modFile, "m", entities.Span{Start: 0, End: 17})
	direct := piece(modFile, "require example.com/direct", entities.Span{Start: 17, End: boundary})
	indirect := piece(
		modFile, "require example.com/transitive", entities.Span{Start: boundary, End: len(body)},
	)
	other := piece("libs/lib/go.mod", "require example.com/direct", entities.Span{})

	edges, err := (importRule{}).Edges(
		t.Context(), snap, []entities.Decl{header, direct, indirect, other},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, direct.Key, indirect.Key) {
		t.Error("the direct require does not wait for the indirect one")
	}

	if edgeBetween(edges, indirect.Key, direct.Key) {
		t.Error("the indirect require waits for the direct one")
	}

	if edgeBetween(edges, other.Key, indirect.Key) {
		t.Error("a require in another go.mod waits for this one's indirect require")
	}
}

// A package an import names is not found until one of its non-test files is in
// the tree, and a test importing a helper package landed 40 commits before the
// helper whenever the reference to it was one the language server could not
// answer. A use waits for the declaration its selector names, and only when it
// names nothing the repository holds for the package's earliest one by history.
// Waiting on an arbitrary declaration put a port that mentions the package in a
// cycle with the implementation the package declares first.
func TestImportRuleMakesAUseWaitForTheDeclarationItNames(t *testing.T) {
	t.Parallel()

	source := "package lead\n\nimport (\n\t\"github.com/org/lib/store\"\n)\n\n" +
		"func Opens() { store.Open() }\n\nfunc Guesses() { store.Missing() }\n\n" +
		"type Port interface{ Close() }\n"

	snap := treeWith(t, map[string]string{
		"apps/lead/x.go":           source,
		"libs/lib/store/a.go":      "package store\n",
		"libs/lib/store/b.go":      "package store\n",
		"libs/lib/store/c_test.go": "package store\n",
	})
	snap.History = []entities.SourceTouch{
		{Position: 2, Rev: "a", Touched: []string{"libs/lib/store/a.go"}},
		{Position: 5, Rev: "b", Touched: []string{"libs/lib/store/b.go"}},
		{Position: 1, Rev: "c", Touched: []string{"libs/lib/store/c_test.go"}},
	}

	opens := withSpan(goDecl("apps/lead/x.go", "Opens"), source, "func Opens", "func Guesses")
	guesses := withSpan(goDecl("apps/lead/x.go", "Guesses"), source, "func Guesses", "type Port")
	port := withSpan(goDecl("apps/lead/x.go", "Port"), source, "type Port", "")
	earliest := goDecl("libs/lib/store/a.go", "Earliest")
	open := goDecl("libs/lib/store/b.go", "Open")
	onlyTest := goDecl("libs/lib/store/c_test.go", "Open")
	decls := []entities.Decl{
		piece("apps/lead/go.mod", "github.com/org/lead", entities.Span{}),
		piece("libs/lib/go.mod", "github.com/org/lib", entities.Span{}),
		opens, guesses, port, earliest, open, onlyTest,
	}

	edges, err := (importRule{}).Edges(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	testCases := []struct {
		name      string
		dependent entities.Decl
		provider  entities.Decl
		want      bool
	}{
		{"a use waits for the declaration it names", opens, open, true},
		{"a use does not wait for one it does not name", opens, earliest, false},
		{"a use never waits for a test declaration", opens, onlyTest, false},
		{"a use naming nothing waits for the earliest", guesses, earliest, true},
		{"a port that never names the package waits for nothing", port, earliest, false},
		{"a port is not tied to what the package declares", port, open, false},
	}

	for _, testCase := range testCases {
		got := edgeBetween(edges, testCase.dependent.Key, testCase.provider.Key)
		if got != testCase.want {
			t.Errorf("%s: edge = %v, want %v", testCase.name, got, testCase.want)
		}
	}
}

// withSpan gives decl the span from the first marker to the second, or to the
// end of source when the second is empty.
func withSpan(decl entities.Decl, source, from, to string) entities.Decl {
	decl.Span = entities.Span{Start: strings.Index(source, from), End: len(source)}
	if to != "" {
		decl.Span.End = strings.Index(source, to)
	}

	return decl
}

// A blank or dot import is kept by a render in every file it writes, so every
// declaration of the file needs the package to exist.
func TestImportRuleMakesEveryDeclarationWaitForABlankImport(t *testing.T) {
	t.Parallel()

	source := "package lead\n\nimport _ \"github.com/org/lib/driver\"\n\nfunc Idle() {}\n"

	snap := treeWith(t, map[string]string{
		"apps/lead/x.go":       source,
		"libs/lib/driver/d.go": "package driver\n",
	})

	idle := goDecl("apps/lead/x.go", "Idle")
	idle.Span = entities.Span{Start: strings.Index(source, "func Idle"), End: len(source)}

	provider := goDecl("libs/lib/driver/d.go", "D")
	decls := []entities.Decl{
		piece("apps/lead/go.mod", "github.com/org/lead", entities.Span{}),
		piece("libs/lib/go.mod", "github.com/org/lib", entities.Span{}),
		idle, provider,
	}

	edges, err := (importRule{}).Edges(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, idle.Key, provider.Key) {
		t.Error("a declaration using no selector does not wait for a blank-imported package")
	}
}

// A file importing one type of a workspace module made every declaration of the
// file wait for that module's require, and so for its manifest, long before the
// module's first package: only the declaration reaching the package waits.
func TestImportRuleMakesOnlyTheDeclarationUsingAPackageWaitForItsRequire(t *testing.T) {
	t.Parallel()

	source := "package lead\n\nimport \"github.com/org/lib/store\"\n\n" +
		"func Uses() { store.Open() }\n\nfunc Idle() {}\n"

	snap := treeWith(t, map[string]string{
		"apps/lead/x.go":      source,
		"apps/lead/go.mod":    "module m\n",
		"libs/lib/store/a.go": "package store\n",
	})

	uses := withSpan(goDecl("apps/lead/x.go", "Uses"), source, "func Uses", "func Idle")
	idle := withSpan(goDecl("apps/lead/x.go", "Idle"), source, "func Idle", "")
	require := piece("apps/lead/go.mod", "require github.com/org/lib", entities.Span{})
	decls := []entities.Decl{
		piece("apps/lead/go.mod", "github.com/org/lead", entities.Span{}),
		piece("libs/lib/go.mod", "github.com/org/lib", entities.Span{}),
		require, uses, idle, goDecl("libs/lib/store/a.go", "Open"),
	}

	edges, err := (importRule{}).Edges(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, uses.Key, require.Key) {
		t.Error("the declaration using the package does not wait for its require")
	}

	if edgeBetween(edges, idle.Key, require.Key) {
		t.Error("a declaration using nothing of the package waits for its require")
	}
}
