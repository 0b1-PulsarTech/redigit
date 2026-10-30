package resolve

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
)

func testdataImporterFiles() map[string]string {
	return map[string]string{
		"main.go": "package fixture\n\nfunc Used() {}\n\ntype Doer interface{ Do() }\n\n" +
			"type Impl struct{}\n\nfunc (Impl) Do() {}\n",
		"testdata/x/x.go": "package x\n\nimport f \"example.test/fixture\"\n\n" +
			"func U() { f.Used(); var d f.Doer; _ = d }\n",
	}
}

func TestTypedReferencesUnderTrimpath(t *testing.T) {
	t.Setenv("GOFLAGS", "-trimpath")

	fix := newFixture(t, testdataImporterFiles())

	testCases := []struct {
		name    string
		query   spot
		wantErr error
	}{
		{"a function it calls", spot{"main.go", "Used", 0}, ErrNotCovered},
		{"an interface it names", spot{"main.go", "Doer", 0}, ErrNotCovered},
		{"an unused type of it", spot{"main.go", "Impl", 0}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := fix.resolver.References(context.Background(), fix.locate(t, testCase.query))
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("References() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedReferencesOfADependencyWithALineDirective(t *testing.T) {
	t.Parallel()

	user := "package x\n\nimport \"example.test/fixture/p\"\n\nfunc U() { p.F(); p.G() }\n"

	testCases := []struct {
		name    string
		dep     string
		query   spot
		wantErr error
	}{
		{
			"a line comment directive",
			"package p\n\n//line gen.go:50\nfunc F() {}\n\nfunc G() {}\n",
			spot{"p/p.go", "G()", 0},
			ErrNotCovered,
		},
		{
			"a line only directive",
			"package p\n\n//line :50\nfunc F() {}\n\nfunc G() {}\n",
			spot{"p/p.go", "F()", 0},
			ErrNotCovered,
		},
		{
			"a general comment directive",
			"package p\n\n/*line gen.go:50*/\nfunc F() {}\n\nfunc G() {}\n",
			spot{"p/p.go", "G()", 0},
			ErrNotCovered,
		},
		{
			"no directive",
			"package p\n\nfunc F() {}\n\nfunc G() {}\n\nfunc H() {}\n",
			spot{"p/p.go", "H()", 0},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := newFixture(t, map[string]string{
				"p/p.go": testCase.dep, "testdata/x/x.go": user,
			})

			_, err := fix.resolver.References(context.Background(), fix.locate(t, testCase.query))
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("References() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedReferencesOfATransitiveDependencyWithALineDirective(t *testing.T) {
	t.Parallel()

	middle := "package p1\n\nimport \"example.test/fixture/p2\"\n\nfunc New() p2.T { return p2.T{} }\n"
	user := "package x\n\nimport \"example.test/fixture/p1\"\n\nfunc U() { p1.New().M() }\n"
	sibling := "package p2\n\nfunc Q1() {}\n\nfunc Q2() {}\n"

	testCases := []struct {
		name    string
		dep     string
		wantErr error
	}{
		{
			"a directive naming a sibling file",
			"package p2\n\ntype T struct{}\n\n//line q.go:2\nfunc (T) M() {}\n",
			ErrNotCovered,
		},
		{
			"a directive naming a template",
			"package p2\n\ntype T struct{}\n\n//line t.tmpl:2\nfunc (T) M() {}\n",
			ErrNotCovered,
		},
		{
			"a directive naming its own file",
			"package p2\n\ntype T struct{}\n\n//line p2.go:2\nfunc (T) M() {}\n",
			ErrNotCovered,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := newFixture(t, map[string]string{
				"p2/p2.go": testCase.dep, "p2/q.go": sibling, "p1/p1.go": middle,
				"testdata/x/x.go": user,
			})

			_, err := fix.resolver.References(
				context.Background(), fix.locate(t, spot{"p2/p2.go", "M()", 0}),
			)
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("References() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedReferencesOfATestFileClause(t *testing.T) {
	t.Parallel()

	importer := "package x\n\nimport \"example.test/fixture/p\"\n\nfunc U() { p.F() }\n"
	pkg := "package p\n\nfunc F() {}\n\nfunc G() {}\n"

	testCases := []struct {
		name    string
		files   map[string]string
		wantErr error
	}{
		{
			"a dependency of another group",
			map[string]string{
				"p/p.go": pkg, "p/p_test.go": "package p\n\nfunc TestH() { G() }\n",
				"testdata/x/x.go": importer,
			},
			ErrNotCovered,
		},
		{
			"a dependency of another group that declares",
			map[string]string{
				"p/p.go": pkg, "p/p_test.go": "package p\n\nfunc H() {}\n\nfunc TestH() { H() }\n",
				"testdata/x/x.go": importer,
			},
			ErrNotCovered,
		},
		{
			"the only group",
			map[string]string{"p/p.go": pkg, "p/p_test.go": "package p\n\nfunc TestH() { G() }\n"},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := newFixture(t, testCase.files)

			_, err := fix.resolver.References(
				context.Background(), fix.locate(t, spot{"p/p_test.go", "p\n", 0}),
			)
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("References() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedReferencesOfAMainPackageATestdataPackageImports(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, testdataImporterFiles())

	testCases := []struct {
		name    string
		query   spot
		wantErr error
	}{
		{"an unused type of it", spot{"main.go", "Impl", 0}, nil},
		{"an unused method of it", spot{"main.go", "Do()", 1}, nil},
		{"a function it calls", spot{"main.go", "Used", 0}, ErrNotCovered},
		{"an interface it names", spot{"main.go", "Doer", 0}, ErrNotCovered},
		{"the clause of it", spot{"main.go", "fixture", 0}, ErrNotCovered},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := fix.resolver.References(context.Background(), fix.locate(t, testCase.query))
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("References() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedReferencesOfAClauseAnImportOutsideTheGroupReads(t *testing.T) {
	t.Parallel()

	helper := map[string]string{
		"a.go":                   "package fixture\n",
		"testdata/helper/doc.go": "package helper\n",
		"testdata/helper/h.go":   "package helper\n\nfunc H() {}\n",
	}

	with := func(name, content string) map[string]string {
		files := maps.Clone(helper)
		files[name] = content

		return files
	}

	testCases := []struct {
		name    string
		files   map[string]string
		query   spot
		wantErr error
	}{
		{
			"a named import of a package the group does not root",
			with(
				"user.go",
				"package fixture\n\nimport \"example.test/fixture/testdata/helper\"\n\n"+
					"func Use() { helper.H() }\n",
			),
			spot{"testdata/helper/doc.go", "helper", 0},
			ErrNotCovered,
		},
		{
			"a blank import of a package the group does not root",
			with(
				"user.go",
				"package fixture\n\nimport _ \"example.test/fixture/testdata/helper\"\n",
			),
			spot{"testdata/helper/doc.go", "helper", 0},
			ErrNotCovered,
		},
		{
			"a testdata package importing the main module",
			with("testdata/user/u.go", "package user\n\nimport _ \"example.test/fixture\"\n"),
			spot{"a.go", "fixture", 0},
			ErrNotCovered,
		},
		{
			"a package nothing imports",
			helper,
			spot{"testdata/helper/doc.go", "helper", 0},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := newFixture(t, testCase.files)

			_, err := fix.resolver.References(context.Background(), fix.locate(t, testCase.query))
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("References() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedReferencesOfAPackageClause(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, goplsFiles())

	imports := []spot{
		{"sub/blank.go", "_ \"example.test", 0},
		{"sub/named.go", "fx \"example.test", 0},
		{"sub/user.go", "\"example.test", 0},
	}

	pkImports := []spot{
		{"pkuser/user.go", "_ \"example.test", 0},
		{"pkuser/x_test.go", "_ \"example.test", 0},
	}

	testCases := []struct {
		name  string
		query spot
		want  []spot
	}{
		{"a file with no declaration", spot{"doc.go", "fixture", 0}, imports},
		{"a file that declares", spot{"cell.go", "fixture", 0}, imports},
		{"a package nothing imports", spot{"sub/user.go", "sub", 0}, nil},
		{"a plain file of a package with tests", spot{"pk/pk.go", "pk", 0}, pkImports},
		{"a test file of the same package", spot{"pk/in_test.go", "pk", 0}, pkImports},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.resolver.References(context.Background(), fix.locate(t, testCase.query))
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
