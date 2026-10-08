package planning

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func sized(file, name string, lines int, generated bool) entities.Decl {
	out := decl(file, name, entities.UnitID(dirOf(file)))
	out.Lines = lines
	out.Generated = generated

	return out
}

func TestTallyOverflows(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		have tally
		add  weight
		want bool
	}{
		{"empty commit takes any declaration", tally{}, weight{lines: 5000}, false},
		{"hand-written under the cap", tally{hand: 400}, weight{lines: 500}, false},
		{"hand-written exactly at the cap", tally{hand: 400}, weight{lines: 500}, false},
		{"hand-written past the cap", tally{hand: 400}, weight{lines: 501}, true},
		{
			"generated never counts toward hand-written",
			tally{hand: 900},
			weight{lines: 50, generated: true},
			false,
		},
		{
			"generated joins source while small",
			tally{hand: 200, generated: 600},
			weight{lines: 300, generated: true},
			false,
		},
		{
			"generated past the cap leaves source out",
			tally{hand: 200, generated: 600},
			weight{lines: 301, generated: true},
			true,
		},
		{
			"source never joins a large generated set",
			tally{generated: 1200},
			weight{lines: 1},
			true,
		},
		{
			"generated joins a large generated set",
			tally{generated: 1200},
			weight{lines: 400, generated: true},
			false,
		},
		{"zero-line declaration never overflows", tally{hand: 900}, weight{}, false},
		{
			"first declaration of a file brings its frame",
			tally{hand: 850},
			weight{file: "a.go", lines: 40, frame: 20},
			true,
		},
		{
			"later declaration of that file does not",
			tally{hand: 850, files: map[string]bool{"a.go": true}},
			weight{file: "a.go", lines: 40, frame: 20},
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.have.overflows(testCase.add, 900); got != testCase.want {
				t.Fatalf("overflows(%+v) = %v, want %v", testCase.add, got, testCase.want)
			}
		})
	}
}

func TestAnnotate(t *testing.T) {
	t.Parallel()

	if got := annotate("why", tally{hand: 900}, 900); got != "why" {
		t.Fatalf("annotate at the cap = %q, want the reason untouched", got)
	}

	if got := annotate(
		"why",
		tally{hand: 1200},
		900,
	); !strings.Contains(
		got,
		"1200 hand-written lines",
	) {
		t.Fatalf("annotate over the cap = %q, want the line count named", got)
	}
}

// cutter plans through both ways the planner groups declarations, because the
// cap has to hold in each.
type cutter struct {
	name string
	plan func(t *testing.T, decls []entities.Decl, edges []entities.Edge) []entities.PlannedCommit
}

func cutters() []cutter {
	order := func(decls []entities.Decl) entities.Ordering {
		at := make(map[entities.DeclKey]entities.Position, len(decls))
		for index, d := range decls {
			at[d.Key] = entities.Position(index)
		}

		return entities.Ordering{At: at}
	}

	return []cutter{
		{
			"capability",
			func(t *testing.T, decls []entities.Decl, edges []entities.Edge) []entities.PlannedCommit {
				t.Helper()

				capabilities := Capabilities{}
				for _, d := range decls {
					capabilities[d.Key] = "slice"
				}

				plan, err := New(Config{Capabilities: capabilities, MaxFiles: 50}).
					Plan(entities.NewGraph(decls, edges, nil), order(decls))
				if err != nil {
					t.Fatalf("Plan() = %v", err)
				}

				return plan.Commits
			},
		},
		{
			"bucket",
			func(t *testing.T, decls []entities.Decl, edges []entities.Edge) []entities.PlannedCommit {
				t.Helper()

				keys := make([]entities.DeclKey, 0, len(decls))
				for _, d := range decls {
					keys = append(keys, d.Key)
				}

				return New(Config{MaxFiles: 50}).cut(
					entities.NewGraph(decls, edges, nil),
					order(decls),
					bucket{
						keys:    keys,
						shape:   entities.ShapeLayer,
						feature: SharedFeature,
						reason:  "test",
					},
				)
			},
		},
	}
}

func handLines(graph entities.Graph, commit entities.PlannedCommit) (hand, generated int) {
	got := tallyOf(graph, commit.Decls)

	return got.hand, got.generated
}

func TestPlannerCutsATestPackageOverTheCap(t *testing.T) {
	t.Parallel()

	files := []struct {
		name  string
		lines int
	}{{"api/a_test.go", 600}, {"api/b_test.go", 600}, {"api/c_test.go", 300}}

	decls := make([]entities.Decl, 0, 3*len(files))

	for _, file := range files {
		for part := range 3 {
			d := sized(file.name, fmt.Sprintf("Test%d", part), file.lines/3, false)
			d.IsTest = true
			decls = append(decls, d)
		}
	}

	for _, run := range cutters() {
		t.Run(run.name, func(t *testing.T) {
			t.Parallel()

			commits := run.plan(t, decls, nil)
			if len(commits) != 2 {
				t.Fatalf("1500 lines became %d commits, want 2", len(commits))
			}

			graph := entities.NewGraph(decls, nil, nil)

			for _, commit := range commits {
				if hand, _ := handLines(graph, commit); hand > 900 {
					t.Fatalf(
						"commit %d adds %d hand-written lines, want at most 900",
						commit.Position,
						hand,
					)
				}
			}
		})
	}
}

func TestPlannerKeepsALargeMockApartFromItsInterface(t *testing.T) {
	t.Parallel()

	decls := make([]entities.Decl, 0, 13)
	decls = append(decls, sized("store/store.go", "Store", 200, false))

	for part := range 12 {
		decls = append(decls, sized("store/store_mock.go", fmt.Sprintf("Mock%d", part), 100, true))
	}

	for _, run := range cutters() {
		t.Run(run.name, func(t *testing.T) {
			t.Parallel()

			commits := run.plan(t, decls, nil)
			if len(commits) != 2 {
				t.Fatalf("got %d commits, want the interface and the mock apart", len(commits))
			}

			graph := entities.NewGraph(decls, nil, nil)

			for _, commit := range commits {
				hand, generated := handLines(graph, commit)
				if hand > 0 && generated > 900 {
					t.Fatalf(
						"commit %d mixes %d hand-written with %d generated lines",
						commit.Position,
						hand,
						generated,
					)
				}
			}
		})
	}
}

func TestPlannerKeepsASmallMockWithItsInterface(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		sized("store/store.go", "Store", 200, false),
		sized("store/store_mock.go", "Mock", 400, true),
	}

	for _, run := range cutters() {
		t.Run(run.name, func(t *testing.T) {
			t.Parallel()

			if commits := run.plan(t, decls, nil); len(commits) != 1 {
				t.Fatalf("got %d commits, want 1: a small mock is not worth a commit", len(commits))
			}
		})
	}
}

func TestPlannerKeepsAnAtomOverTheCapWholeAndSaysSo(t *testing.T) {
	t.Parallel()

	first := sized("kinds/a.go", "Kind", 600, false)
	first.Atom = "kind"
	second := sized("kinds/b.go", "Kind.Error", 600, false)
	second.Atom = "kind"

	decls := []entities.Decl{first, second, sized("kinds/c.go", "Other", 10, false)}

	for _, run := range cutters() {
		t.Run(run.name, func(t *testing.T) {
			t.Parallel()

			commits := run.plan(t, decls, nil)
			if len(commits) != 2 {
				t.Fatalf("got %d commits, want the atom whole and the rest apart", len(commits))
			}

			if len(commits[0].Decls) != 2 ||
				!strings.Contains(commits[0].Reason, "over the 900 cap") {
				t.Fatalf(
					"commit 0 = %v (%q), want the atom whole and reported",
					commits[0].Decls,
					commits[0].Reason,
				)
			}
		})
	}
}

// A cut can fall anywhere the cap says. It must never fall inside an atom, and
// never put a declaration in a commit earlier than one it needs.
func TestCapNeverSeparatesAnAtomOrOrdersAgainstADependency(t *testing.T) {
	t.Parallel()

	var (
		decls = make([]entities.Decl, 0, 60)
		edges = make([]entities.Edge, 0, 59)
	)

	for index := range 60 {
		d := sized(
			fmt.Sprintf("lib/f%d.go", index%7),
			fmt.Sprintf("D%d", index),
			70+index%5*40,
			index%11 == 0,
		)
		if index%4 < 2 {
			d.Atom = fmt.Sprintf("atom%d", index/4)
		}

		decls = append(decls, d)

		if index > 0 {
			edges = append(edges, needs(d, decls[index-1]))
		}
	}

	graph := entities.NewGraph(decls, edges, nil)

	for _, run := range cutters() {
		t.Run(run.name, func(t *testing.T) {
			t.Parallel()

			commits := run.plan(t, decls, edges)
			commitOf := map[entities.DeclKey]int{}
			atomIn := map[string]int{}

			for index, commit := range commits {
				for _, key := range commit.Decls {
					commitOf[key] = index

					atom := mustDecl(t, graph, key).Atom
					if home, seen := atomIn[atom]; atom != "" && seen && home != index {
						t.Fatalf("atom %s is in commits %d and %d", atom, home, index)
					}

					atomIn[atom] = index
				}
			}

			for _, edge := range edges {
				if commitOf[edge.Provider] > commitOf[edge.Dependent] {
					t.Fatalf(
						"%s sits in commit %d before its provider in %d",
						edge.Dependent,
						commitOf[edge.Dependent],
						commitOf[edge.Provider],
					)
				}
			}

			if len(commitOf) != len(decls) {
				t.Fatalf("%d of %d declarations placed", len(commitOf), len(decls))
			}
		})
	}
}

func mustDecl(t *testing.T, graph entities.Graph, key entities.DeclKey) entities.Decl {
	t.Helper()

	found, ok := graph.Decl(key)
	if !ok {
		t.Fatalf("%s is not in the graph", key)
	}

	return found
}
