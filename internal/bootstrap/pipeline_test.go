package bootstrap

import (
	"cmp"
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

// TestPipelineCloseFoldsBothFailures pins the defect this task exists to
// prevent from recurring silently: Close used to stop at the first failure,
// so a checkout left behind by a broken session close was never reported.
func TestPipelineCloseFoldsBothFailures(t *testing.T) {
	t.Parallel()

	errSession := errors.New("language server would not stop")
	errReader := errors.New("cat-file would not stop")
	errCheckout := errors.New("checkout is still locked")
	errLoads := errors.New("typed load would not stop")

	testCases := []struct {
		name     string
		loads    fakeCloser
		session  fakeCloser
		reader   fakeCloser
		checkout fakeCloser
		want     []error
	}{
		{name: "all close cleanly"},
		{
			name:  "only the typed loads fail",
			loads: fakeCloser{err: errLoads},
			want:  []error{errLoads},
		},
		{
			name:    "only the session fails",
			session: fakeCloser{err: errSession},
			want:    []error{errSession},
		},
		{
			name:     "only the checkout fails",
			checkout: fakeCloser{err: errCheckout},
			want:     []error{errCheckout},
		},
		{
			name:     "the session fails, so the checkout it may still read is left alone",
			session:  fakeCloser{err: errSession},
			checkout: fakeCloser{err: errCheckout},
			want:     []error{errSession},
		},
		{
			name:   "only the git reader fails",
			reader: fakeCloser{err: errReader},
			want:   []error{errReader},
		},
		{
			name:     "the reader fails beside a session that keeps the checkout",
			session:  fakeCloser{err: errSession},
			reader:   fakeCloser{err: errReader},
			checkout: fakeCloser{err: errCheckout},
			want:     []error{errSession, errReader},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pipeline := &Pipeline{
				loads:    testCase.loads,
				session:  testCase.session,
				reader:   testCase.reader,
				checkout: testCase.checkout,
			}

			err := pipeline.Close(t.Context())
			if len(testCase.want) == 0 && err != nil {
				t.Fatalf("Close() = %v, want nil", err)
			}

			for _, target := range []error{errLoads, errSession, errReader, errCheckout} {
				if errors.Is(err, target) != slices.Contains(testCase.want, target) {
					t.Fatalf("Close() = %v, want exactly %v wrapped", err, testCase.want)
				}
			}
		})
	}
}

type orderedCloser struct {
	name  string
	order *[]string
}

func (closing orderedCloser) Close(context.Context) error {
	*closing.order = append(*closing.order, closing.name)

	return nil
}

func TestPipelineCloseStopsTheResolverBeforeTheLoadsAndTheCheckoutGoes(t *testing.T) {
	t.Parallel()

	var order []string

	pipeline := &Pipeline{
		loads:    orderedCloser{"loads", &order},
		session:  orderedCloser{"session", &order},
		reader:   orderedCloser{"reader", &order},
		checkout: orderedCloser{"checkout", &order},
	}

	if err := pipeline.Close(t.Context()); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	want := []string{"session", "loads", "reader", "checkout"}
	if !slices.Equal(order, want) {
		t.Fatalf("Close() stopped %v, want %v", order, want)
	}
}

func TestPipelineCloseKeepsTheCheckoutWhenLoadsStay(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		loadsErr   error
		sessionErr error
		wantOrder  []string
		wantKept   bool
	}{
		{"the loads stop", nil, nil, []string{"session", "loads", "reader", "checkout"}, false},
		{
			"the close context expired",
			context.DeadlineExceeded,
			nil,
			[]string{"session", "loads", "reader"},
			true,
		},
		{
			"the session did not close",
			nil,
			context.DeadlineExceeded,
			[]string{"session", "loads", "reader"},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var order []string

			pipeline := &Pipeline{
				loads:    failingCloser{orderedCloser{"loads", &order}, testCase.loadsErr},
				session:  failingCloser{orderedCloser{"session", &order}, testCase.sessionErr},
				reader:   orderedCloser{"reader", &order},
				checkout: orderedCloser{"checkout", &order},
				snapshot: entities.Snapshot{Root: "/tmp/kept-checkout"},
			}

			err := pipeline.Close(t.Context())

			if !slices.Equal(order, testCase.wantOrder) {
				t.Fatalf("Close() stopped %v, want %v", order, testCase.wantOrder)
			}

			if testCase.wantKept {
				if !errors.Is(err, cmp.Or(testCase.loadsErr, testCase.sessionErr)) ||
					!strings.Contains(err.Error(), "/tmp/kept-checkout") {
					t.Fatalf("Close() = %v, want the kept checkout path and the close failure", err)
				}
			} else if err != nil {
				t.Fatalf("Close() = %v, want nil", err)
			}
		})
	}
}

type failingCloser struct {
	orderedCloser

	err error
}

func (closing failingCloser) Close(ctx context.Context) error {
	_ = closing.orderedCloser.Close(ctx)

	return closing.err
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

func TestUnclaimed(t *testing.T) {
	t.Parallel()

	langs := []indexing.Language{
		{Name: "go", Extensions: []string{".go"}},
		{Name: "config", Extensions: []string{".mod", ".yml"}},
	}

	testCases := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "every final-tree path is owned by some language",
			files: []string{"main.go", "go.mod"},
			want:  nil,
		},
		{
			name:  "a path no language's extension list matches is reported",
			files: []string{"main.go", "engines/wwebjs/src/index.ts", "database/atlas.hcl"},
			want:  []string{"engines/wwebjs/src/index.ts", "database/atlas.hcl"},
		},
		{
			name:  "an empty final tree reports nothing",
			files: nil,
			want:  nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap := entities.Snapshot{Files: testCase.files}

			got := unclaimed(snap, langs)
			if len(got) != len(testCase.want) {
				t.Fatalf("unclaimed() = %+v, want %d entries", got, len(testCase.want))
			}

			for i, file := range testCase.want {
				if got[i].File != file {
					t.Fatalf("unclaimed()[%d].File = %q, want %q", i, got[i].File, file)
				}
			}
		})
	}
}

func TestBuildFlags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		tags []string
		want []string
	}{
		{"no tags yields nil, not -tags=", nil, nil},
		{"empty slice also yields nil", []string{}, nil},
		{"one tag", []string{"integration"}, []string{"-tags=integration"}},
		{
			"multiple tags share one flag, as go keeps only the last",
			[]string{"integration", "e2e"},
			[]string{"-tags=integration,e2e"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := buildFlags(testCase.tags)

			if testCase.want == nil {
				if got != nil {
					t.Fatalf("buildFlags(%v) = %v, want nil", testCase.tags, got)
				}

				return
			}

			if len(got) != len(testCase.want) {
				t.Fatalf("buildFlags(%v) = %v, want %v", testCase.tags, got, testCase.want)
			}

			for i, flag := range testCase.want {
				if got[i] != flag {
					t.Fatalf("buildFlags(%v)[%d] = %q, want %q", testCase.tags, i, got[i], flag)
				}
			}
		})
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

func TestRunHooksReleasesWhatEarlierHooksOpenedWhenOneFails(t *testing.T) {
	t.Parallel()

	boom := errors.New("hook failed")

	testCases := []struct {
		name       string
		failing    error
		wantClosed []string
	}{
		{
			name:       "a failing hook closes the earlier hooks' closers once",
			failing:    boom,
			wantClosed: []string{"b", "a"},
		},
		{name: "no failure leaves them open for the pipeline", wantClosed: nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var order []string

			opening := func(name string, err error) namedHook {
				return namedHook{name: name, run: func(
					_ context.Context, _ *Config, _ stageEnv, built *stages,
				) error {
					built.closers = append(built.closers, orderedCloser{name, &order})

					return err
				}}
			}

			cfg := Defaults()
			hooks := []namedHook{opening("a", nil), opening("b", nil), {name: "last", run: func(
				context.Context, *Config, stageEnv, *stages,
			) error {
				return testCase.failing
			}}}

			err := runHooks(t.Context(), &cfg, stageEnv{}, identityStages(nil), hooks)
			if !errors.Is(err, testCase.failing) {
				t.Fatalf("runHooks() = %v, want %v", err, testCase.failing)
			}

			if !slices.Equal(order, testCase.wantClosed) {
				t.Fatalf("closed %v, want %v", order, testCase.wantClosed)
			}
		})
	}
}

func TestPipelineCloseStopsTheHookResourcesFirst(t *testing.T) {
	t.Parallel()

	errHook := errors.New("hook process would not stop")

	testCases := []struct {
		name      string
		hookErr   error
		wantOrder []string
	}{
		{
			name:      "before the session and the checkout they may read",
			wantOrder: []string{"hook", "session", "loads", "reader", "checkout"},
		},
		{
			name:      "a hook closer's error surfaces",
			hookErr:   errHook,
			wantOrder: []string{"hook", "session", "loads", "reader", "checkout"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var order []string

			pipeline := &Pipeline{
				hooked:   closers{failingCloser{orderedCloser{"hook", &order}, testCase.hookErr}},
				loads:    orderedCloser{"loads", &order},
				session:  orderedCloser{"session", &order},
				reader:   orderedCloser{"reader", &order},
				checkout: orderedCloser{"checkout", &order},
			}

			err := pipeline.Close(t.Context())

			if !errors.Is(err, testCase.hookErr) {
				t.Fatalf("Close() = %v, want %v", err, testCase.hookErr)
			}

			if !slices.Equal(order, testCase.wantOrder) {
				t.Fatalf("Close() stopped %v, want %v", order, testCase.wantOrder)
			}
		})
	}
}
