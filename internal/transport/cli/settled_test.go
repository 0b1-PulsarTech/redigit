package cli

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
)

// TestSettled pins settled's own switch: it must fail when the ordering
// never converged or carries no positions, must fail when the fixed point
// ran no rounds, and must NOT fail on a cycle or an unresolved declaration -
// those come with a usable ordering and are warnings, not failures.
func TestSettled(t *testing.T) {
	t.Parallel()

	errGraph := errors.New("resolver unreachable")
	graph := declGraph()

	testCases := []settledTestCase{
		{
			name:       "the graph stage fails, order is never called",
			graphErr:   errGraph,
			callsOrder: false,
			wantErr:    errGraph,
		},
		{
			name:       "a fixed point that did not converge is fatal",
			order:      entities.Ordering{At: map[entities.DeclKey]entities.Position{}, Rounds: 0},
			orderErr:   ordering.ErrNotConverged,
			callsOrder: true,
			wantErr:    ordering.ErrNotConverged,
		},
		{
			name:       "no positions at all is fatal even with no error",
			order:      entities.Ordering{At: nil, Rounds: 5},
			callsOrder: true,
			wantErr:    apperr.ErrIncomplete,
		},
		{
			name: "a fixed point that ran zero rounds is fatal",
			order: entities.Ordering{
				At:     map[entities.DeclKey]entities.Position{declKey("A"): 0},
				Rounds: 0,
			},
			callsOrder: true,
			wantErr:    apperr.ErrIncomplete,
		},
		{
			name: "a cycle comes with a usable ordering and is a warning",
			order: entities.Ordering{
				At:     map[entities.DeclKey]entities.Position{declKey("A"): 0},
				Rounds: 3,
				Cycles: []entities.Cycle{{Members: []entities.DeclKey{declKey("A")}}},
			},
			orderErr:    apperr.ErrIncomplete,
			callsOrder:  true,
			wantSuccess: true,
		},
		{
			name: "an unresolved declaration comes with a usable ordering and is a warning",
			order: entities.Ordering{
				At:     map[entities.DeclKey]entities.Position{declKey("A"): 0},
				Rounds: 3,
			},
			orderErr:    &ordering.UnresolvedError{Declarations: []entities.DeclKey{declKey("A")}},
			callsOrder:  true,
			wantSuccess: true,
		},
		{
			name: "a clean convergence succeeds with no complaint",
			order: entities.Ordering{
				At:     map[entities.DeclKey]entities.Position{declKey("A"): 0},
				Rounds: 3,
			},
			callsOrder:  true,
			wantSuccess: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runSettledTestCase(t, graph, testCase)
		})
	}
}

type settledTestCase struct {
	name        string
	graphErr    error
	order       entities.Ordering
	orderErr    error
	callsOrder  bool
	wantErr     error
	wantSuccess bool
}

func runSettledTestCase(t *testing.T, graph entities.Graph, testCase settledTestCase) {
	t.Helper()

	ctrl := gomock.NewController(t)
	pl := NewMockPipeline(ctrl)
	pl.EXPECT().Unseen().Return(nil).AnyTimes()
	pl.EXPECT().Unclaimed().Return(nil).AnyTimes()

	if testCase.graphErr != nil {
		pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{}, testCase.graphErr)
	} else {
		pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{Graph: graph}, nil)
	}

	if testCase.callsOrder {
		pl.EXPECT().Order(gomock.Any()).Return(testCase.order, testCase.orderErr)
	}

	_, order, err := settled(context.Background(), pl)

	if testCase.wantSuccess {
		if err != nil {
			t.Fatalf("settled() err = %v, want nil", err)
		}

		if order.Rounds != testCase.order.Rounds {
			t.Fatalf("Rounds = %d, want %d", order.Rounds, testCase.order.Rounds)
		}

		return
	}

	if !errors.Is(err, testCase.wantErr) {
		t.Fatalf("settled() err = %v, want it to wrap %v", err, testCase.wantErr)
	}
}
