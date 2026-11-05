package bootstrap

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// fakeCloser, fakeIndexer, fakeScanner, fakeOrderer, fakePlanner, and
// fakeAuditor each satisfy a one-method local port. Hand-rolled rather than
// generated: every port here has a single method, below the mockgen
// threshold this repo uses for behavioural dependency mocks.
type fakeCloser struct{ err error }

func (f fakeCloser) Close(context.Context) error { return f.err }

type fakeIndexer struct {
	result indexing.Result
	err    error
}

func (f fakeIndexer) Index(context.Context, entities.Snapshot) (indexing.Result, error) {
	return f.result, f.err
}

type fakeScanner struct {
	births indexing.Births
	err    error
}

func (f fakeScanner) Scan(context.Context, entities.Snapshot) (indexing.Births, error) {
	return f.births, f.err
}

type fakeOrderer struct {
	order entities.Ordering
	err   error
}

func (f fakeOrderer) Order(entities.Graph, entities.Snapshot) (entities.Ordering, error) {
	return f.order, f.err
}

type fakePlanner struct {
	plan entities.Plan
	err  error
}

func (f fakePlanner) Plan(entities.Graph, entities.Ordering) (entities.Plan, error) {
	return f.plan, f.err
}

type fakeAuditor struct {
	result auditing.Result
	err    error
}

func (f fakeAuditor) Audit(entities.Graph, entities.Ordering) (auditing.Result, error) {
	return f.result, f.err
}

var (
	_ closer  = fakeCloser{}
	_ indexer = fakeIndexer{}
	_ scanner = fakeScanner{}
	_ orderer = fakeOrderer{}
	_ planner = fakePlanner{}
	_ auditor = fakeAuditor{}
)

func TestPipelineSnapshot(t *testing.T) {
	t.Parallel()

	want := entities.Snapshot{Root: "/repo"}
	pipeline := &Pipeline{snapshot: want}

	if got := pipeline.Snapshot(); got.Root != want.Root {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestPipelineGraph(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("resolver unreachable")

	testCases := []struct {
		name    string
		indexer fakeIndexer
		wantErr error
	}{
		{
			name:    "wraps the indexer's error",
			indexer: fakeIndexer{err: errBoom},
			wantErr: errBoom,
		},
		{
			name:    "returns the result unchanged on success",
			indexer: fakeIndexer{result: indexing.Result{Graph: entities.NewGraph(nil, nil, nil)}},
			wantErr: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pipeline := &Pipeline{indexer: testCase.indexer}

			_, err := pipeline.Graph(t.Context())
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestPipelineOrder(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("fixed point exploded")
	graph := entities.NewGraph(nil, nil, nil)

	testCases := []struct {
		name    string
		orderer fakeOrderer
		wantErr error
	}{
		{
			name:    "wraps the orderer's error",
			orderer: fakeOrderer{err: errBoom},
			wantErr: errBoom,
		},
		{
			name:    "returns the ordering unchanged on success",
			orderer: fakeOrderer{order: entities.Ordering{Rounds: 3}},
			wantErr: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pipeline := &Pipeline{orderer: testCase.orderer}

			got, err := pipeline.Order(graph)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			// The ordering is returned even alongside an error: a caller
			// reports on a cycle or an unresolved declaration, it does not
			// just discard the result.
			if testCase.wantErr == nil && got.Rounds != testCase.orderer.order.Rounds {
				t.Fatalf("Rounds = %d, want %d", got.Rounds, testCase.orderer.order.Rounds)
			}
		})
	}
}

func TestPipelinePlan(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("cannot group")
	graph := entities.NewGraph(nil, nil, nil)
	order := entities.Ordering{}

	testCases := []struct {
		name    string
		planner fakePlanner
		wantErr error
	}{
		{
			name:    "wraps the planner's error",
			planner: fakePlanner{err: errBoom},
			wantErr: errBoom,
		},
		{
			name:    "returns the plan unchanged on success",
			planner: fakePlanner{plan: entities.Plan{Unplaced: []entities.DeclKey{{Name: "x"}}}},
			wantErr: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pipeline := &Pipeline{planner: testCase.planner}

			_, err := pipeline.Plan(graph, order)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestPipelineAudit(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("nothing to audit")
	graph := entities.NewGraph(nil, nil, nil)
	order := entities.Ordering{}

	testCases := []struct {
		name    string
		auditor fakeAuditor
		wantErr error
	}{
		{
			name:    "wraps the auditor's error",
			auditor: fakeAuditor{err: errBoom},
			wantErr: errBoom,
		},
		{
			name:    "returns the result unchanged on success",
			auditor: fakeAuditor{result: auditing.Result{Drifts: []auditing.Drift{{}}}},
			wantErr: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pipeline := &Pipeline{auditor: testCase.auditor}

			_, err := pipeline.Audit(graph, order)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestPipelineBirths(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("no history")

	testCases := []struct {
		name    string
		scanner fakeScanner
		wantErr error
	}{
		{
			name:    "wraps the scanner's error",
			scanner: fakeScanner{err: errBoom},
			wantErr: errBoom,
		},
		{
			name: "returns the births unchanged on success",
			scanner: fakeScanner{births: indexing.Births{
				At: map[entities.DeclKey]entities.Position{{Name: "x"}: 1},
			}},
			wantErr: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pipeline := &Pipeline{scanner: testCase.scanner}

			_, err := pipeline.Births(t.Context())
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestPipelineUnclaimed(t *testing.T) {
	t.Parallel()

	want := []auditing.Unclaimed{{File: "database/atlas.hcl"}}
	pipeline := &Pipeline{unclaimed: want}

	got := pipeline.Unclaimed()
	if len(got) != len(want) || got[0].File != want[0].File {
		t.Fatalf("Unclaimed() = %+v, want %+v", got, want)
	}
}

func TestRunHooks(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	replaced := errors.New("replaced")
	kept := []string{"kept"}

	testCases := []struct {
		name          string
		hooks         []hookSpec
		wantTrace     []string
		wantErr       error
		wantPrefix    string
		wantLanguages []string
		wantReplaced  bool
	}{
		{
			name:          "hooks run in order",
			hooks:         []hookSpec{{name: "rules"}, {name: "shaping"}, {name: "lineage"}},
			wantTrace:     []string{"rules", "shaping", "lineage"},
			wantLanguages: kept,
		},
		{name: "no hooks leave the stages alone", wantLanguages: kept},
		{
			name: "the first error short-circuits and names the hook",
			hooks: []hookSpec{
				{name: "rules"}, {name: "shaping", err: boom}, {name: "lineage"},
			},
			wantTrace:     []string{"rules", "shaping"},
			wantErr:       boom,
			wantPrefix:    "shaping: ",
			wantLanguages: kept,
		},
		{
			name:          "a hook can replace the planner, the orderer and the languages",
			hooks:         []hookSpec{{name: "shaping", replace: true}},
			wantTrace:     []string{"shaping"},
			wantLanguages: []string{"kept", "added"},
			wantReplaced:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var trace []string

			cfg := Defaults()
			built := identityStages(nil)

			hooks := buildHooks(&trace, testCase.hooks, replaced)
			err := runHooks(t.Context(), &cfg, stageEnv{}, built, hooks)

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("runHooks() err = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.HasPrefix(err.Error(), testCase.wantPrefix) {
				t.Fatalf("runHooks() err = %q, want prefix %q", err, testCase.wantPrefix)
			}

			if !slices.Equal(trace, testCase.wantTrace) {
				t.Fatalf("trace = %v, want %v", trace, testCase.wantTrace)
			}

			names, swapped := summarise(built, replaced)

			if !slices.Equal(names, testCase.wantLanguages) {
				t.Fatalf("languages = %v, want %v", names, testCase.wantLanguages)
			}

			if swapped != testCase.wantReplaced {
				t.Fatalf("stages replaced = %v, want %v", swapped, testCase.wantReplaced)
			}
		})
	}
}

type hookSpec struct {
	name    string
	err     error
	replace bool
}

func buildHooks(trace *[]string, specs []hookSpec, replaced error) []namedHook {
	hooks := make([]namedHook, 0, len(specs))

	for _, spec := range specs {
		hooks = append(hooks, namedHook{name: spec.name, run: func(
			_ context.Context, _ *Config, _ stageEnv, built *stages,
		) error {
			*trace = append(*trace, spec.name)

			if spec.replace {
				built.planner = fakePlanner{err: replaced}
				built.orderer = fakeOrderer{err: replaced}
				built.languages = append(built.languages, indexing.Language{Name: "added"})
			}

			return spec.err
		}})
	}

	return hooks
}

func summarise(built *stages, replaced error) ([]string, bool) {
	names := make([]string, 0, len(built.languages))
	for _, language := range built.languages {
		names = append(names, language.Name)
	}

	_, orderErr := built.orderer.Order(entities.Graph{}, entities.Snapshot{})
	_, planErr := built.planner.Plan(entities.Graph{}, entities.Ordering{})

	return names, errors.Is(orderErr, replaced) && errors.Is(planErr, replaced)
}

func identityStages(failure error) *stages {
	return &stages{
		languages: []indexing.Language{{Name: "kept"}},
		orderer:   fakeOrderer{err: failure},
		planner:   fakePlanner{err: failure},
	}
}

func hookLeavesTheStagesUnchanged(t *testing.T, hook stageHook) {
	t.Helper()

	kept := errors.New("kept")
	cfg := Defaults()
	built := identityStages(kept)

	if err := hook(t.Context(), &cfg, stageEnv{}, built); err != nil {
		t.Fatalf("hook() = %v, want nil", err)
	}

	if len(built.languages) != 1 || built.languages[0].Name != "kept" {
		t.Fatalf("languages = %v, want the one kept language", built.languages)
	}

	_, orderErr := built.orderer.Order(entities.Graph{}, entities.Snapshot{})
	_, planErr := built.planner.Plan(entities.Graph{}, entities.Ordering{})

	if !errors.Is(orderErr, kept) || !errors.Is(planErr, kept) {
		t.Fatalf("orderer err = %v, planner err = %v, want both kept", orderErr, planErr)
	}

	if cfg.Rev != Defaults().Rev {
		t.Fatalf("cfg.Rev = %q, want %q", cfg.Rev, Defaults().Rev)
	}
}
