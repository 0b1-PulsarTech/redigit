package ordering

import (
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestFindCyclesSplitsAtHostsEdges(t *testing.T) {
	t.Parallel()

	fn := hostFn
	mainFn, runFn, twin, other := fn("main.go", "main"), fn("run.go", "run"),
		fn("run.go", "twin"), fn("other.go", "other")
	manifest := entities.DeclKey{File: "web/package.json", Kind: entities.KindOther, Name: "<file>"}
	script := entities.DeclKey{File: "web/src/index.ts", Kind: entities.KindOther, Name: "<file>"}
	decls := []entities.Decl{
		{Key: mainFn}, {Key: runFn}, {Key: twin}, {Key: other}, {Key: manifest}, {Key: script},
	}

	edge := hostEdge
	ref, host := entities.ReasonReference, entities.ReasonHosts

	testCases := []cycleCase{
		{
			"a cross-file cycle closed only by a hosts edge is not a cycle",
			[]entities.Edge{edge(ref, mainFn, runFn), edge(host, runFn, mainFn)},
			[]string{},
		},
		{
			"a hosts-closed cycle keeps the single-file recursion inside it",
			[]entities.Edge{
				edge(ref, mainFn, runFn), edge(host, runFn, mainFn),
				edge(ref, runFn, twin), edge(ref, twin, runFn),
			},
			[]string{"run,twin"},
		},
		{
			"a cycle that crosses files without the hosts edge stays whole",
			[]entities.Edge{
				edge(ref, mainFn, runFn), edge(host, runFn, mainFn),
				edge(ref, runFn, other), edge(ref, other, runFn),
			},
			[]string{"main,other,run"},
		},
		{
			"a cycle through a hosts edge onto a manifest is still reported",
			[]entities.Edge{edge(host, script, manifest), edge(ref, manifest, script)},
			[]string{"<file>,<file>"},
		},
		{
			"a single-file cycle closed only by hosts edges is not a cycle",
			[]entities.Edge{edge(ref, runFn, twin), edge(host, twin, runFn)},
			[]string{},
		},
		{
			"a cycle with no hosts edge is untouched",
			[]entities.Edge{edge(ref, mainFn, runFn), edge(ref, runFn, mainFn)},
			[]string{"main,run"},
		},
	}

	checkCycles(t, decls, testCases)
}

func TestFindCyclesSplitsRecursionsInTwoFiles(t *testing.T) {
	t.Parallel()

	apple, avocado := hostFn("a.go", "apple"), hostFn("a.go", "avocado")
	berry, blue := hostFn("b.go", "berry"), hostFn("b.go", "blue")
	decls := []entities.Decl{{Key: apple}, {Key: avocado}, {Key: berry}, {Key: blue}}
	edge := hostEdge
	ref, host := entities.ReasonReference, entities.ReasonHosts

	testCases := []cycleCase{
		{
			"two recursions in two files linked only by hosts edges stay two cycles",
			[]entities.Edge{
				edge(ref, apple, avocado), edge(ref, avocado, apple),
				edge(ref, berry, blue), edge(ref, blue, berry),
				edge(host, apple, berry), edge(host, berry, apple),
			},
			[]string{"apple,avocado", "berry,blue"},
		},
		{
			"two recursions in two files linked by a non-hosts edge stay whole",
			[]entities.Edge{
				edge(ref, apple, avocado), edge(ref, avocado, apple),
				edge(ref, berry, blue), edge(ref, blue, berry),
				edge(ref, apple, berry), edge(ref, berry, apple),
				edge(host, avocado, blue),
			},
			[]string{"apple,avocado,berry,blue"},
		},
	}

	checkCycles(t, decls, testCases)
}

type cycleCase struct {
	name  string
	edges []entities.Edge
	want  []string
}

func hostFn(file, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name}
}

func hostEdge(reason entities.EdgeReason, dependent, provider entities.DeclKey) entities.Edge {
	return entities.Edge{Dependent: dependent, Provider: provider, Reason: reason}
}

func checkCycles(t *testing.T, decls []entities.Decl, testCases []cycleCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := cycleNames(findCycles(entities.NewGraph(decls, testCase.edges, nil)))
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("findCycles() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func cycleNames(cycles []entities.Cycle) []string {
	got := make([]string, 0, len(cycles))

	for _, cycle := range cycles {
		names := make([]string, 0, len(cycle.Members))
		for _, member := range cycle.Members {
			names = append(names, member.Name)
		}

		slices.Sort(names)
		got = append(got, strings.Join(names, ","))
	}

	slices.Sort(got)

	return got
}

func TestOrderHostsClosedCycleIsCoBornAndNotReported(t *testing.T) {
	t.Parallel()

	mainFn := entities.DeclKey{File: "main.go", Kind: entities.KindFunc, Name: "main"}
	runFn := entities.DeclKey{File: "run.go", Kind: entities.KindFunc, Name: "run"}

	testCases := []struct {
		name      string
		edges     []entities.Edge
		wantCycle int
		wantErr   bool
	}{
		{
			"main reads a function that waits for main: co-born, no complaint",
			[]entities.Edge{
				{Dependent: mainFn, Provider: runFn, Reason: entities.ReasonReference},
				{Dependent: runFn, Provider: mainFn, Reason: entities.ReasonHosts},
			},
			0,
			false,
		},
		{
			"the same pair closed by a reference is still reported",
			[]entities.Edge{
				{Dependent: mainFn, Provider: runFn, Reason: entities.ReasonReference},
				{Dependent: runFn, Provider: mainFn, Reason: entities.ReasonReference},
			},
			1,
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(
				[]entities.Decl{{Key: mainFn}, {Key: runFn}},
				testCase.edges,
				nil,
			)
			snap := entities.Snapshot{History: []entities.SourceTouch{
				{Position: 4, Touched: []string{"run.go"}},
				{Position: 74, Touched: []string{"main.go"}},
			}}

			result, err := New(Config{}).Order(graph, snap)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("Order() err = %v, want error %v", err, testCase.wantErr)
			}

			if len(result.Cycles) != testCase.wantCycle {
				t.Fatalf("Cycles = %v, want %d", result.Cycles, testCase.wantCycle)
			}

			if mainAt, runAt := result.Of(mainFn), result.Of(runFn); mainAt != runAt {
				t.Fatalf("main at %d, run at %d, want them co-born", mainAt, runAt)
			}
		})
	}
}
