package golang

import (
	"maps"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestRulePackPlansAPackageMainBornWithItsHelpers(t *testing.T) {
	t.Parallel()

	mainFn := testkit.DeclKey("main.go", entities.KindFunc, "main")
	runFn := testkit.DeclKey("main.go", entities.KindFunc, "run")
	splitRun := testkit.DeclKey("run.go", entities.KindFunc, "run")
	splitUsage := testkit.DeclKey("run.go", entities.KindValue, "exitUsage")
	libFiles := map[string]string{
		"lib/lib.go": "package lib\n\nfunc Do() {}\n",
		"run.go": "package main\n\nimport \"example.test/fixture/lib\"\n\n" +
			"const exitUsage = 2\n\nfunc run() int {\n\tlib.Do()\n\n\treturn exitUsage\n}\n",
		"main.go": "package main\n\nimport \"os\"\n\nfunc main() {\n\tos.Exit(run())\n}\n",
	}
	spare := maps.Clone(libFiles)
	spare["lib/lib.go"] += "\nfunc Spare() {}\n"

	testCases := []struct {
		name    string
		files   map[string]string
		touches []entities.SourceTouch
		born    []entities.DeclKey
	}{
		{
			"main calls a function of its own file",
			map[string]string{
				"main.go": "package main\n\nfunc main() {\n\trun()\n}\n\nfunc run() {}\n",
			},
			[]entities.SourceTouch{{Position: 1, Touched: []string{"main.go"}}},
			[]entities.DeclKey{mainFn, runFn},
		},
		{
			"main calls a function of another file",
			libFiles,
			[]entities.SourceTouch{
				{Position: 1, Touched: []string{"lib/lib.go"}},
				{Position: 4, Touched: []string{"run.go"}},
				{Position: 74, Touched: []string{"main.go"}},
			},
			[]entities.DeclKey{mainFn, splitRun, splitUsage},
		},
		{
			"an unreferenced library function does not take main into its feature",
			spare,
			[]entities.SourceTouch{
				{Position: 1, Touched: []string{"lib/lib.go"}},
				{Position: 4, Touched: []string{"run.go"}},
				{Position: 74, Touched: []string{"main.go"}},
			},
			[]entities.DeclKey{mainFn, splitRun, splitUsage},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph, result := orderedFixture(t, testCase.files, testCase.touches, nil)

			plan, err := planning.New(planning.Config{}).Plan(graph, result)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			first := plannedCommitOf(plan, testCase.born[0])

			for _, key := range testCase.born {
				commit := plannedCommitOf(plan, key)
				if commit.Position != first.Position || commit.Feature != "." {
					t.Fatalf(
						"%v is in feature %q at %d, want feature \".\" at %d with main",
						key, commit.Feature, commit.Position, first.Position,
					)
				}
			}
		})
	}
}

func plannedCommitOf(plan entities.Plan, key entities.DeclKey) entities.PlannedCommit {
	for _, commit := range plan.Commits {
		if slices.Contains(commit.Decls, key) {
			return commit
		}
	}

	return entities.PlannedCommit{Position: entities.NoPosition, Feature: "unplanned"}
}

func TestRulePackHostsNeverDelayTheHead(t *testing.T) {
	t.Parallel()

	head := testkit.DeclKey("main.go", entities.KindFunc, "main")
	runFn := testkit.DeclKey("run.go", entities.KindFunc, "run")
	usage := testkit.DeclKey("run.go", entities.KindValue, "exitUsage")
	statement := testkit.DeclKey("main.go", entities.KindStmt, "main#1")

	testCases := []struct {
		name     string
		files    map[string]string
		touches  []entities.SourceTouch
		wantHead entities.Position
		after    []entities.DeclKey
	}{
		{
			"a sliced main whose helper is touched later",
			map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
				"run.go":  "package main\n\nfunc run() {}\n",
			},
			[]entities.SourceTouch{
				{Position: 2, Touched: []string{"main.go"}},
				{Position: 8, Touched: []string{"run.go"}},
			},
			2,
			[]entities.DeclKey{runFn, statement},
		},
		{
			"a sliced main whose helper and constant were touched earlier",
			map[string]string{
				"main.go": "package main\n\nimport \"os\"\n\nfunc main() {\n\tos.Exit(run())\n}\n",
				"run.go":  "package main\n\nconst exitUsage = 2\n\nfunc run() int { return exitUsage }\n",
			},
			[]entities.SourceTouch{
				{Position: 4, Touched: []string{"run.go"}},
				{Position: 74, Touched: []string{"main.go"}},
			},
			74,
			[]entities.DeclKey{runFn, usage, statement},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph, result := orderedFixture(
				t, testCase.files, testCase.touches, sliceTheHead(head, runFn, statement),
			)

			plan, err := planning.New(planning.Config{}).Plan(graph, result)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			if result.Of(head) != testCase.wantHead {
				t.Fatalf("head at %d, want %d", result.Of(head), testCase.wantHead)
			}

			headAt := plannedCommitOf(plan, head).Position
			if headAt == entities.NoPosition {
				t.Fatal("the head is not in the plan")
			}

			for _, key := range testCase.after {
				at := plannedCommitOf(plan, key).Position
				if at == entities.NoPosition {
					t.Fatalf("%v is not in the plan", key)
				}

				if at < headAt {
					t.Errorf("%v planned at %d, before the head at %d", key, at, headAt)
				}

				if result.Of(key) < testCase.wantHead {
					t.Errorf("%v ordered before the head", key)
				}
			}
		})
	}
}

func sliceTheHead(
	head, callee, statement entities.DeclKey,
) func([]entities.Decl, []entities.Edge) ([]entities.Decl, []entities.Edge) {
	return func(decls []entities.Decl, edges []entities.Edge) ([]entities.Decl, []entities.Edge) {
		edges = slices.DeleteFunc(edges, func(edge entities.Edge) bool {
			return edge.Dependent == head && edge.Provider == callee
		})

		return append(decls, entities.Decl{Key: statement, Unit: "."}), append(
			edges,
			entities.Edge{Dependent: statement, Provider: callee, Reason: entities.ReasonReference},
			entities.Edge{Dependent: statement, Provider: head, Reason: entities.ReasonBody},
		)
	}
}
