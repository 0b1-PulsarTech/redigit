package bootstrap

import (
	"context"
	"errors"
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
	errCheckout := errors.New("checkout is still locked")

	testCases := []struct {
		name         string
		session      fakeCloser
		checkout     fakeCloser
		wantSession  bool
		wantCheckout bool
		wantNil      bool
	}{
		{
			name:     "both close cleanly",
			session:  fakeCloser{},
			checkout: fakeCloser{},
			wantNil:  true,
		},
		{
			name:        "only the session fails",
			session:     fakeCloser{err: errSession},
			checkout:    fakeCloser{},
			wantSession: true,
		},
		{
			name:         "only the checkout fails",
			session:      fakeCloser{},
			checkout:     fakeCloser{err: errCheckout},
			wantCheckout: true,
		},
		{
			name:         "both fail, and neither hides the other",
			session:      fakeCloser{err: errSession},
			checkout:     fakeCloser{err: errCheckout},
			wantSession:  true,
			wantCheckout: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pipeline := &Pipeline{session: testCase.session, checkout: testCase.checkout}

			err := pipeline.Close(t.Context())

			if testCase.wantNil {
				if err != nil {
					t.Fatalf("Close() = %v, want nil", err)
				}

				return
			}

			if testCase.wantSession && !errors.Is(err, errSession) {
				t.Fatalf("Close() = %v, want it to wrap %v", err, errSession)
			}

			if !testCase.wantSession && errors.Is(err, errSession) {
				t.Fatalf("Close() = %v, did not expect it to wrap %v", err, errSession)
			}

			if testCase.wantCheckout && !errors.Is(err, errCheckout) {
				t.Fatalf("Close() = %v, want it to wrap %v", err, errCheckout)
			}

			if !testCase.wantCheckout && errors.Is(err, errCheckout) {
				t.Fatalf("Close() = %v, did not expect it to wrap %v", err, errCheckout)
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
			"multiple tags each get their own flag",
			[]string{"integration", "e2e"},
			[]string{"-tags=integration", "-tags=e2e"},
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
