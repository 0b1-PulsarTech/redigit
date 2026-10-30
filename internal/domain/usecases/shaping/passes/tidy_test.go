package passes

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

type (
	declSpec struct {
		file      string
		name      string
		kind      entities.DeclKind
		lines     int
		test      bool
		generated bool
		iface     bool
		atom      string
	}

	edgeSpec struct{ dependent, provider string }

	linkSpec struct {
		edgeSpec

		reason entities.EdgeReason
	}

	tidyCase struct {
		name         string
		rows         [][]declSpec
		edges        []edgeSpec
		links        []linkSpec
		config       string
		runs         int
		wantSubjects []string
		wantMoves    int
	}

	nopObserver struct{}
)

func (nopObserver) Moved(string, shaping.Move) {}

func (nopObserver) Refused(string, []entities.Edge) {}

func synth(rows [][]declSpec, edges []edgeSpec, links ...linkSpec) *shaping.Draft {
	var (
		decls []entities.Decl
		plan  entities.Plan
	)

	keys := map[string]entities.DeclKey{}

	for index, row := range rows {
		commit := entities.PlannedCommit{
			Position: entities.Position(index), Subject: "row " + string(rune('a'+index)),
		}

		for _, spec := range row {
			key := entities.DeclKey{
				File: spec.file, Kind: cmp.Or(spec.kind, entities.KindFunc), Name: spec.name,
			}
			keys[spec.name] = key
			decls = append(decls, entities.Decl{
				Key: key, Lines: spec.lines, IsTest: spec.test, Generated: spec.generated,
				IsInterface: spec.iface, Atom: spec.atom,
			})

			commit.Decls = append(commit.Decls, key)
		}

		plan.Commits = append(plan.Commits, commit)
	}

	links = slices.Clip(links)
	for _, edge := range edges {
		links = append(links, linkSpec{edge, entities.ReasonReference})
	}

	graphEdges := make([]entities.Edge, 0, len(links))
	for _, link := range links {
		graphEdges = append(graphEdges, entities.Edge{
			Dependent: keys[link.dependent], Provider: keys[link.provider], Reason: link.reason,
		})
	}

	return shaping.NewDraft(entities.NewGraph(decls, graphEdges, nil), plan)
}

func foreignCases() []tidyCase {
	provider := declSpec{file: "pkg/b/b.go", name: "B", lines: 20}
	host := declSpec{file: "pkg/a/a.go", name: "A", lines: 50}
	foreign := declSpec{file: "pkg/b/b_test.go", name: "TestB", lines: 250, test: true}
	small := declSpec{file: "pkg/b/b_test.go", name: "TestB", lines: 100, test: true}
	testsB := []edgeSpec{{"TestB", "B"}}
	moved := []string{"row a", "DRAFT: test(b): b_test.go", "row b"}

	return []tidyCase{
		{
			name:         "a foreign-package test run moves after its provider",
			rows:         [][]declSpec{{provider}, {host, foreign}},
			edges:        testsB,
			wantSubjects: moved,
			wantMoves:    1,
		},
		{
			name:         "an already-placed run stays",
			rows:         [][]declSpec{{provider}, {foreign}, {host}},
			edges:        testsB,
			wantSubjects: []string{"row a", "row b", "row c"},
		},
		{
			name:         "a run below the line threshold stays",
			rows:         [][]declSpec{{provider}, {host, small}},
			edges:        testsB,
			wantSubjects: []string{"row a", "row b"},
		},
		{
			name:         "a shorter threshold moves a smaller run",
			rows:         [][]declSpec{{provider}, {host, small}},
			edges:        testsB,
			config:       `{"type":"tidy-test-after","min_test_lines":50}`,
			wantSubjects: moved,
			wantMoves:    1,
		},
		{
			name:         "a run with no provider stays",
			rows:         [][]declSpec{{host, foreign}},
			wantSubjects: []string{"row a"},
		},
		{
			name:         "a second run makes zero moves",
			rows:         [][]declSpec{{provider}, {host, foreign}},
			edges:        testsB,
			runs:         2,
			wantSubjects: moved,
			wantMoves:    1,
		},
	}
}

func homeCases() []tidyCase {
	provider := declSpec{file: "pkg/b/b.go", name: "B", lines: 20}
	manifest := declSpec{file: "go.mod", name: "mod", kind: entities.KindOther, lines: 30}
	bigTest := declSpec{file: "pkg/a/a_test.go", name: "TestA", lines: 250, test: true}
	stays := []string{"row a", "row b"}

	return []tidyCase{
		{
			name: "a block of pure production code never moves under a test subject",
			rows: [][]declSpec{
				{provider},
				{
					{file: "pkg/a/a.go", name: "A", lines: 60},
					{file: "pkg/b/x.go", name: "X", lines: 10},
				},
			},
			edges:        []edgeSpec{{"X", "B"}},
			config:       `{"type":"tidy-test-after","min_test_lines":1}`,
			wantSubjects: stays,
		},
		{
			name: "a module manifest does not take the package's home from its code",
			rows: [][]declSpec{
				{provider},
				{manifest, {file: "pkg/a/a.go", name: "A", lines: 10}, bigTest},
			},
			edges:        []edgeSpec{{"TestA", "B"}},
			wantSubjects: stays,
		},
		{
			name:         "a commit of only manifest and tests has no home to be foreign to",
			rows:         [][]declSpec{{provider}, {manifest, bigTest}},
			edges:        []edgeSpec{{"TestA", "B"}},
			wantSubjects: stays,
		},
	}
}

func shapeCases() []tidyCase {
	provider := declSpec{file: "pkg/b/b.go", name: "B", lines: 20}
	mock := declSpec{
		file: "pkg/b/mock_test.go", name: "MockB", lines: 60, test: true, generated: true,
	}
	filler := func(name string) declSpec {
		return declSpec{file: "pkg/c/c.go", name: name, lines: 30}
	}

	return []tidyCase{
		{
			name: "trailing mocks move after the code they fake",
			rows: [][]declSpec{
				{provider}, {filler("C1")}, {filler("C2")}, {filler("C3")}, {mock},
			},
			edges:        []edgeSpec{{"MockB", "B"}},
			wantSubjects: []string{"row a", "row e", "row b", "row c", "row d"},
			wantMoves:    1,
		},
		{
			name: "a whole trailing test commit keeps its human subject",
			rows: [][]declSpec{
				{provider},
				{filler("C1")},
				{filler("C2")},
				{{file: "pkg/b/b_test.go", name: "TestB", lines: 90, test: true}},
			},
			edges:        []edgeSpec{{"TestB", "B"}},
			wantSubjects: []string{"row a", "row d", "row b", "row c"},
			wantMoves:    1,
		},
		{
			name: "an emptied slot before a test commit does not hide that it is placed",
			rows: [][]declSpec{
				{provider},
				{{file: "pkg/d/d.go", name: "D", lines: 20}},
				{filler("F")},
				{
					{file: "pkg/b/b_test.go", name: "TestB", lines: 90, test: true},
					{file: "pkg/d/d_test.go", name: "TestD", lines: 90, test: true},
				},
				{{file: "pkg/e/e_test.go", name: "TestE", lines: 90, test: true}},
			},
			edges: []edgeSpec{{"TestB", "B"}, {"TestD", "D"}, {"TestE", "F"}},
			runs:  2,
			wantSubjects: []string{
				"row a", "DRAFT: test(b): b_test.go", "row b", "DRAFT: test(d): d_test.go",
				"row c", "row e",
			},
		},
		{
			name:         "a trailing run already after its provider stays",
			rows:         [][]declSpec{{provider}, {mock}, {filler("C1")}},
			edges:        []edgeSpec{{"MockB", "B"}},
			wantSubjects: []string{"row a", "row b", "row c"},
		},
		{
			name: "a move that would violate is refused",
			rows: [][]declSpec{{
				{file: "pkg/c/c.go", name: "C", lines: 10},
				{file: "pkg/a/a.go", name: "A", lines: 100},
				{file: "pkg/a/h.go", name: "H", lines: 5},
				{file: "pkg/b/x_test.go", name: "TestX", lines: 250, test: true},
			}},
			edges:        []edgeSpec{{"TestX", "C"}, {"H", "TestX"}},
			wantSubjects: []string{"row a"},
		},
	}
}

func windowCases() []tidyCase {
	const (
		total    = 20
		early    = 7
		wantBoth = 2
	)

	rows := make([][]declSpec, total)
	edges := []edgeSpec{{"T1", "B"}, {"T2", "B"}}
	subjects := []string{"row a", "row h", "row t"}

	for index := range rows {
		name := "F" + strconv.Itoa(index)
		rows[index] = []declSpec{{file: "pkg/c/c.go", name: name, lines: 30}}

		if index != 0 && index != early && index != total-1 {
			subjects = append(subjects, "row "+string(rune('a'+index)))
		}
	}

	rows[0] = []declSpec{{file: "pkg/b/b.go", name: "B", lines: 20}}
	rows[early] = []declSpec{{file: "pkg/b/t1_test.go", name: "T1", lines: 90, test: true}}
	rows[total-1] = []declSpec{{file: "pkg/b/t2_test.go", name: "T2", lines: 90, test: true}}

	return []tidyCase{
		{
			name:         "a test commit just outside the window moves once the window shifts",
			rows:         rows,
			edges:        edges,
			wantSubjects: subjects,
			wantMoves:    wantBoth,
		},
		{
			name:         "a second run over the same plan makes zero moves",
			rows:         rows,
			edges:        edges,
			runs:         2,
			wantSubjects: subjects,
		},
	}
}

func TestTidyApply(t *testing.T) {
	t.Parallel()

	for _, testCase := range slices.Concat(
		foreignCases(), homeCases(), shapeCases(), windowCases(), reviewCases(),
	) {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := cmp.Or(testCase.config, `{"type":"tidy-test-after"}`)

			tidy, err := NewTidy(json.RawMessage(config))
			if err != nil {
				t.Fatalf("NewTidy() = %v", err)
			}

			draft := synth(testCase.rows, testCase.edges, testCase.links...)
			last, wantLast := 0, testCase.wantMoves

			for range max(testCase.runs, 1) {
				moves, applyErr := tidy.Apply(t.Context(), draft)
				if applyErr != nil {
					t.Fatalf("Apply() = %v", applyErr)
				}

				last = len(moves)
			}

			if testCase.runs > 1 {
				wantLast = 0
			}

			plan := draft.Plan()

			got := make([]string, 0, len(plan.Commits))
			for _, commit := range plan.Commits {
				got = append(got, commit.Subject)
			}

			if !slices.Equal(got, testCase.wantSubjects) || last != wantLast {
				t.Fatalf(
					"subjects = %v, last run %d moves, want %v, %d",
					got, last, testCase.wantSubjects, wantLast,
				)
			}

			if violations := draft.Violations(); len(violations) != 0 {
				t.Fatalf("Apply() left violations: %v", violations)
			}
		})
	}
}

func TestTidyApplyStopsOnCancel(t *testing.T) {
	t.Parallel()

	tidy, err := NewTidy(json.RawMessage(`{"type":"tidy-test-after"}`))
	if err != nil {
		t.Fatalf("NewTidy() = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	draft := synth([][]declSpec{{{file: "a.go", name: "A", lines: 1}}}, nil)
	if _, err = tidy.Apply(ctx, draft); !errors.Is(err, context.Canceled) {
		t.Fatalf("Apply() = %v, want context.Canceled", err)
	}
}

func TestNewTidy(t *testing.T) {
	t.Parallel()

	const (
		base  = `{"type":"tidy-test-after"`
		knobs = `,"min_test_lines":10,"max_prod_lines":5,"tail_window":3}`
	)

	testCases := []struct {
		name    string
		raw     string
		want    TidyConfig
		wantErr error
	}{
		{"defaults", base + `}`, TidyConfig{200, 40, 12}, nil},
		{"every knob", base + knobs, TidyConfig{10, 5, 3}, nil},
		{"unknown field", base + `,"nope":1}`, TidyConfig{}, shaping.ErrMalformed},
		{"wrong type", base + `,"tail_window":"x"}`, TidyConfig{}, shaping.ErrMalformed},
		{"negative knob", base + `,"min_test_lines":-1}`, TidyConfig{}, shaping.ErrMalformed},
		{"zero min_test_lines", base + `,"min_test_lines":0}`, TidyConfig{}, shaping.ErrMalformed},
		{"zero tail_window", base + `,"tail_window":0}`, TidyConfig{200, 40, 0}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tidy, err := NewTidy(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("NewTidy() error = %v, want %v", err, testCase.wantErr)
			}

			if err == nil && tidy.cfg != testCase.want {
				t.Fatalf("NewTidy() config = %+v, want %+v", tidy.cfg, testCase.want)
			}
		})
	}
}

func benchPlan(packages, perPackage int) ([][]declSpec, []edgeSpec) {
	rows := make([][]declSpec, 0, packages)
	edges := make([]edgeSpec, 0, packages*perPackage*2)

	for pkg := range packages {
		prod := make([]declSpec, 0, perPackage)

		for index := range perPackage {
			name := "P" + strconv.Itoa(pkg) + "_" + strconv.Itoa(index)
			prod = append(prod, declSpec{
				file: "pkg/p" + strconv.Itoa(pkg) + "/f.go", name: name, lines: 30,
			})

			if index > 0 {
				previous := "P" + strconv.Itoa(pkg) + "_" + strconv.Itoa(index-1)
				edges = append(edges, edgeSpec{name, previous})
			}
		}

		rows = append(rows, prod)
	}

	for pkg := range packages {
		name := "T" + strconv.Itoa(pkg)
		host := (pkg + 7) % packages
		rows[host] = append(rows[host], declSpec{
			file: "pkg/p" + strconv.Itoa(pkg) + "/f_test.go", name: name, lines: 300, test: true,
		})

		edges = append(edges, edgeSpec{name, "P" + strconv.Itoa(pkg) + "_0"})
	}

	return rows, edges
}

func BenchmarkPipelineTidy(b *testing.B) {
	tidy, err := NewTidy(json.RawMessage(`{"type":"tidy-test-after"}`))
	if err != nil {
		b.Fatal(err)
	}

	pipeline := shaping.NewPipeline(nopObserver{}, tidy)
	base := synth(benchPlan(100, 20))

	var moved int

	b.ReportAllocs()

	for b.Loop() {
		moves, applyErr := pipeline.Apply(b.Context(), base.Clone())
		if applyErr != nil {
			b.Fatal(applyErr)
		}

		moved = len(moves)
	}

	b.ReportMetric(float64(moved), "moves")
}
