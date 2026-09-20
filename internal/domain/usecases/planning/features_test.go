package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func entryPointNames(graph entities.Graph) []string {
	entries := entryPoints(graph)
	got := make([]string, 0, len(entries))

	for _, entry := range entries {
		got = append(got, entry.Key.Name)
	}

	slices.Sort(got)

	return got
}

func TestEntryPoints(t *testing.T) {
	t.Parallel()

	handler := decl("api/handler.go", "Handle", "api")
	service := decl("svc/service.go", "Serve", "svc")
	suite := testDecl("api/handler_test.go", "TestHandle", "api")

	testCases := []struct {
		name  string
		graph entities.Graph
		want  []string
	}{
		{
			"a declaration nothing consumes is a tip",
			entities.NewGraph([]entities.Decl{handler}, nil, nil),
			[]string{"Handle"},
		},
		{
			"a consumed declaration is not",
			entities.NewGraph(
				[]entities.Decl{handler, service},
				[]entities.Edge{needs(handler, service)},
				nil,
			),
			[]string{"Handle"},
		},
		{
			"a test consumer does not disqualify: counting tests would make every tested function its own feature",
			entities.NewGraph(
				[]entities.Decl{handler, suite},
				[]entities.Edge{needs(suite, handler)},
				nil,
			),
			[]string{"Handle"},
		},
		{
			"a test is never itself a tip",
			entities.NewGraph([]entities.Decl{suite}, nil, nil),
			nil,
		},
		{
			"empty graph",
			entities.NewGraph(nil, nil, nil),
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := entryPointNames(testCase.graph); !slices.Equal(got, testCase.want) {
				t.Fatalf("entryPoints() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestEntryPointsExcludesOrphanProse(t *testing.T) {
	t.Parallel()

	handler := decl("api/handler.go", "Handle", "api")
	service := decl("svc/service.go", "Serve", "svc")
	docPage := entities.Decl{
		Key:  entities.DeclKey{File: "docs/PLANO.md", Kind: entities.KindOther, Name: "<file>"},
		Unit: "docs",
	}

	testCases := []struct {
		name  string
		graph entities.Graph
		want  []string
	}{
		{
			"a whole-file declaration with no edge in either direction is prose, not a tip",
			entities.NewGraph([]entities.Decl{handler, docPage}, nil, nil),
			[]string{"Handle"},
		},
		{
			"a structural-only edge - every file in a module carries one - still reads as prose",
			entities.NewGraph(
				[]entities.Decl{handler, docPage, service},
				[]entities.Edge{
					{Dependent: docPage.Key, Provider: service.Key, Reason: entities.ReasonModule},
				},
				nil,
			),
			[]string{"Handle"},
		},
		{
			"a real need - a blank import, say - keeps a whole-file declaration a tip",
			entities.NewGraph(
				[]entities.Decl{handler, docPage, service},
				[]entities.Edge{needs(docPage, service)},
				nil,
			),
			[]string{"<file>", "Handle"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := entryPointNames(testCase.graph); !slices.Equal(got, testCase.want) {
				t.Fatalf("entryPoints() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestFeatureOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		decl entities.Decl
		want FeatureID
	}{
		{"named unit", decl("lead/api/h.go", "Handle", "lead/api"), "lead/api"},
		{
			"an external test unit keeps the directory, so it is the same feature",
			decl("lead/api/h.go", "Handle", "lead/api#api_test"),
			"lead/api",
		},
		{
			"no unit falls back to the file's directory",
			decl("lead/api/h.go", "Handle", ""),
			"lead/api",
		},
		{"a root file", decl("main.go", "Main", ""), "."},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := featureOf(testCase.decl); got != testCase.want {
				t.Fatalf("featureOf() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestReachFrom(t *testing.T) {
	t.Parallel()

	top := decl("a/top.go", "Top", "a")
	mid := decl("a/mid.go", "Mid", "a")
	leaf := decl("a/leaf.go", "Leaf", "a")
	loop := decl("a/loop.go", "Loop", "a")
	unrelated := decl("b/other.go", "Other", "b")

	graph := entities.NewGraph(
		[]entities.Decl{top, mid, leaf, loop, unrelated},
		[]entities.Edge{
			needs(top, mid),
			needs(mid, leaf),
			needs(leaf, loop),
			needs(loop, mid),
		},
		nil,
	)

	testCases := []struct {
		name  string
		start entities.Decl
		want  []string
	}{
		{
			"the whole closure, following providers",
			top,
			[]string{"Leaf", "Loop", "Mid", "Top"},
		},
		{
			"a cycle terminates instead of looping forever",
			loop,
			[]string{"Leaf", "Loop", "Mid"},
		},
		{"a declaration that needs nothing reaches only itself", unrelated, []string{"Other"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var got []string
			for key := range reachFrom(graph, testCase.start.Key) {
				got = append(got, key.Name)
			}

			slices.Sort(got)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("reachFrom() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestParentDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want string
	}{
		{"nested", "a/b/c.go", "a/b"},
		{"root file", "main.go", "."},
		{"leading slash is not a parent", "/main.go", "."},
		{"empty", "", "."},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := parentDir(testCase.path); got != testCase.want {
				t.Fatalf("parentDir(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}
