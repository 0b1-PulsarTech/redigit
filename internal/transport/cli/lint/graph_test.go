package lint

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/stmtread"
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
	"github.com/wrapped-owls/redigit/internal/transport/cli/session"
)

func splitCutFixture() (entities.Graph, stmtread.Reader) {
	source := "package a\n\nfunc run() {\n\terr = g()\n\tstep()\n\tif err != nil {\n\t}\n}\n"
	span := func(snippet string) entities.Span {
		start := strings.Index(source, snippet)

		return entities.Span{Start: start, End: start + len(snippet)}
	}

	head := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "run"}
	stmts := []entities.DeclKey{
		{File: "a.go", Kind: entities.KindStmt, Name: "run#0"},
		{File: "a.go", Kind: entities.KindStmt, Name: "run#1"},
		{File: "a.go", Kind: entities.KindStmt, Name: "run#2"},
	}

	graph := entities.NewGraph(
		[]entities.Decl{
			{Key: head, Unit: "a"},
			{Key: stmts[0], Unit: "a", Span: span("err = g()")},
			{Key: stmts[1], Unit: "a", Span: span("step()")},
			{Key: stmts[2], Unit: "a", Span: span("if err != nil {\n\t}")},
		},
		[]entities.Edge{
			{Dependent: stmts[0], Provider: head, Reason: entities.ReasonBody},
			{Dependent: stmts[1], Provider: head, Reason: entities.ReasonBody},
			{Dependent: stmts[2], Provider: head, Reason: entities.ReasonBody},
		}, nil,
	)

	return graph, stmtread.New(graph, func(context.Context, string) ([]byte, error) {
		return []byte(source), nil
	})
}

func TestMeasureGraphReadsASplitCutFunction(t *testing.T) {
	t.Parallel()

	graph, syntax := splitCutFixture()

	testCases := []struct {
		name         string
		rows         []manifest.Row
		wantMeasures map[string]int
	}{
		{
			name: "red: a void function cut across a wide gap",
			rows: []manifest.Row{
				chunkRow(0, "a.go:func:run", "a.go:stmt:run#0", "a.go:stmt:run#1"),
				chunkRow(30, "a.go:stmt:run#2"),
			},
			wantMeasures: map[string]int{
				linting.MeasureShellsWithoutTail: 1,
				linting.MeasureStubGap:           1,
				linting.MeasureErrcheckSplit:     1,
				linting.MeasureUnusedAtBirth:     1,
			},
		},
		{
			name: "green: the same function whole in one row",
			rows: []manifest.Row{
				chunkRow(
					0,
					"a.go:func:run",
					"a.go:stmt:run#0",
					"a.go:stmt:run#1",
					"a.go:stmt:run#2",
				),
			},
			wantMeasures: map[string]int{linting.MeasureUnusedAtBirth: 1},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			report, err := measureGraph(
				context.Background(), graph, testCase.rows, syntax, linting.DefaultLimits(),
			)
			if err != nil {
				t.Fatalf("measureGraph() err = %v", err)
			}

			got := map[string]int{}
			for _, finding := range report.Findings {
				got[finding.Measure]++
			}

			if len(got) != len(testCase.wantMeasures) {
				t.Fatalf("findings = %v, want %v", got, testCase.wantMeasures)
			}

			for measure, count := range testCase.wantMeasures {
				if got[measure] != count {
					t.Fatalf("findings = %v, want %v", got, testCase.wantMeasures)
				}
			}

			if report.Graph == nil || report.Graph.CutFunctions != 1 {
				t.Fatalf("Graph = %+v, want one cut function", report.Graph)
			}
		})
	}
}

func TestMeasureGraphDoesNotReadAFileOfAnotherLanguageAsGo(t *testing.T) {
	t.Parallel()

	screen := entities.DeclKey{File: "lib/screen.dart", Kind: entities.KindFunc, Name: "build"}
	other := entities.DeclKey{File: "lib/state.dart", Kind: entities.KindFunc, Name: "load"}
	graph := entities.NewGraph(
		[]entities.Decl{{Key: screen, Unit: "lib"}, {Key: other, Unit: "lib"}}, nil, nil,
	)
	syntax := stmtread.New(graph, func(context.Context, string) ([]byte, error) {
		return nil, errBroken
	})

	report, err := measureGraph(
		context.Background(), graph,
		[]manifest.Row{
			chunkRow(0, "lib/screen.dart:func:build"), chunkRow(40, "lib/state.dart:func:load"),
		},
		syntax, linting.DefaultLimits(),
	)
	if err != nil {
		t.Fatalf("measureGraph() err = %v", err)
	}

	if report.Graph == nil || report.Graph.CutFunctions != 0 || len(report.Measures) != 5 {
		t.Fatalf("Graph = %+v, measures = %d", report.Graph, len(report.Measures))
	}

	for _, finding := range report.Findings {
		if finding.Measure != linting.MeasureUnusedAtBirth {
			t.Fatalf("finding = %+v, want only the exempt-or-unused reading", finding)
		}
	}

	if got := measureDetail(report, linting.MeasureUnusedAtBirth); !strings.Contains(
		got, "excluded: not Go 2",
	) {
		t.Fatalf("Detail = %q, want both declarations excluded as not Go", got)
	}
}

type pipelineCase struct {
	name      string
	args      []string
	startErr  error
	graphErr  error
	commitErr error
	wantErr   error
	wantRepo  string
	wantRev   string
	wantTags  []string
}

func TestCheckStructureStartsTheSessionPipeline(t *testing.T) {
	t.Parallel()

	runPipelineCases(t, []pipelineCase{
		{
			name:     "the flags reach the pipeline config",
			args:     []string{"--repo", "/r", "--rev", "v1", "--tags", "a,b", "--mode", "lineage"},
			wantRepo: "/r", wantRev: "v1", wantTags: []string{"a", "b"},
		},
		{
			name:     "a run without --from-cache starts the revision it was given",
			args:     []string{"--repo", "/r", "--rev", "v1"},
			wantRepo: "/r", wantRev: "v1",
		},
		{
			name:     "--from-cache pins the revision to the commit the cache key names",
			args:     []string{"--repo", "/r", "--rev", "v1", "--from-cache"},
			wantRepo: "/r", wantRev: "abc",
		},
	})
}

func TestCheckStructureReportsWhatFailsBeforeTheGraph(t *testing.T) {
	t.Parallel()

	runPipelineCases(t, []pipelineCase{
		{
			name: "a config error from the start is passed through",
			args: []string{"--repo", "/r"}, startErr: bootstrap.ErrConfig,
			wantErr: bootstrap.ErrConfig,
		},
		{
			name: "any other start failure is wrapped", args: []string{"--repo", "/r"},
			startErr: errBroken, wantErr: errBroken,
		},
		{
			name: "a graph that cannot be built", args: []string{"--repo", "/r"},
			graphErr: errBroken, wantErr: errBroken,
		},
		{
			name: "a revision that cannot be resolved", args: []string{"--repo", "/r"},
			commitErr: errBroken, wantErr: errBroken,
		},
	})
}

func runPipelineCases(t *testing.T, testCases []pipelineCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var started bootstrap.Config

			closed := 0
			env := session.Env{
				Start: func(_ context.Context, cfg bootstrap.Config) (session.Pipeline, error) {
					started = cfg

					return fakePipeline{
						result: indexing.Result{Graph: entities.NewGraph(nil, nil, nil)},
						err:    testCase.graphErr, closed: &closed,
					}, testCase.startErr
				},
				CacheDir:  t.TempDir(),
				BuildID:   func() (string, error) { return "build", nil },
				Commit:    func(context.Context, string, string) (string, error) { return "abc", testCase.commitErr },
				Digest:    func(string) (string, error) { return "d", nil },
				Toolchain: func(context.Context, string, string) (string, error) { return "go", nil },
			}

			opts, err := parse(append([]string{"--manifest", "m"}, testCase.args...))
			if err != nil {
				t.Fatalf("parse() err = %v", err)
			}

			report, err := checkStructure(
				context.Background(), opts, []manifest.Row{chunkRow(0)},
				linting.DefaultLimits(), env,
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("checkStructure() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				return
			}

			if started.Repo != testCase.wantRepo || started.Rev != testCase.wantRev ||
				report.Graph.Commit != "abc" || len(started.BuildTags) != len(testCase.wantTags) {
				t.Fatalf("started = %+v, report = %+v", started, report.Graph)
			}

			if closed != 1 {
				t.Fatalf("pipeline closed %d times, want 1", closed)
			}
		})
	}
}

func errSplitDetails(tb testing.TB, snippets []string) []string {
	tb.Helper()

	source := "package a\n\nfunc run() {\n\t" + strings.Join(snippets, "\n\t") + "\n}\n"
	head := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "run"}
	decls := make([]entities.Decl, 0, 1+len(snippets))
	edges := make([]entities.Edge, 0, len(snippets))
	rows := make([]manifest.Row, 0, len(snippets))

	decls = append(decls, entities.Decl{Key: head, Unit: "a"})

	for index, snippet := range snippets {
		key := entities.DeclKey{
			File: "a.go", Kind: entities.KindStmt, Name: "run#" + strconv.Itoa(index),
		}
		start := strings.Index(source, snippet)
		span := entities.Span{Start: start, End: start + len(snippet)}

		decls = append(decls, entities.Decl{Key: key, Unit: "a", Span: span})
		edges = append(
			edges,
			entities.Edge{Dependent: key, Provider: head, Reason: entities.ReasonBody},
		)

		rows = append(rows, chunkRow(index, key.String()))
	}

	rows[0] = chunkRow(0, head.String(), decls[1].Key.String())
	graph := entities.NewGraph(decls, edges, nil)
	syntax := stmtread.New(graph, func(context.Context, string) ([]byte, error) {
		return []byte(source), nil
	})

	report, err := measureGraph(context.Background(), graph, rows, syntax, linting.DefaultLimits())
	if err != nil {
		tb.Fatalf("measureGraph() err = %v", err)
	}

	var details []string

	for _, finding := range report.Findings {
		if finding.Measure == linting.MeasureErrcheckSplit {
			details = append(details, finding.Detail)
		}
	}

	return details
}

func TestMeasureGraphNamesWhatTheErrcheckSplitReads(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		snippets   []string
		wantDetail string
	}{
		{
			name:       "err declared with var is already bound",
			snippets:   []string{"var err error", "x, err := g()", "if err != nil {\n\t}"},
			wantDetail: "err already bound, with other names defined at statement 1",
		},
		{
			name:       "the first x, err := binds err here",
			snippets:   []string{"pieces, err := cut()", "if err != nil {\n\t}"},
			wantDetail: "err newly bound here at statement 0",
		},
		{
			name:       "var a, err = g() is a pair that binds err here",
			snippets:   []string{"var a, err = g()", "if err != nil {\n\t}"},
			wantDetail: "err newly bound here at statement 0",
		},
		{
			name:       "a plain err = is named as one",
			snippets:   []string{"var err error", "err = g()", "if err != nil {\n\t}"},
			wantDetail: "plain err = at statement 1",
		},
		{
			name: "a comment before the assignment and before the check",
			snippets: []string{
				"// why\n\tx, err := g()", "// then\n\tif err != nil {\n\t}",
			},
			wantDetail: "err newly bound here at statement 0",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			details := errSplitDetails(t, testCase.snippets)

			if len(details) != 1 || !strings.Contains(details[0], testCase.wantDetail) {
				t.Fatalf("errcheck_split findings = %q, want one holding %q",
					details, testCase.wantDetail)
			}
		})
	}
}

type fakePipeline struct {
	session.Pipeline

	result indexing.Result
	err    error
	closed *int
}

func (fake fakePipeline) Graph(context.Context) (indexing.Result, error) {
	return fake.result, fake.err
}

func (fake fakePipeline) Close(context.Context) error {
	*fake.closed++

	return nil
}

func measureDetail(report linting.Report, name string) string {
	for _, measure := range report.Measures {
		if measure.Name == name {
			return measure.Detail
		}
	}

	return ""
}
