package linting

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type cutCase struct {
	name      string
	spec      structureSpec
	wantTier  GraphTier
	wantErr   error
	wantNamed string
}

func runCutCases(t *testing.T, testCases []cutCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			report, err := CheckStructure(
				context.Background(),
				structureOf(testCase.spec, syntaxMock{}),
				DefaultLimits(),
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("CheckStructure() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				if !strings.Contains(err.Error(), testCase.wantNamed) {
					t.Fatalf("err = %q, want it to name %q", err, testCase.wantNamed)
				}

				return
			}

			if *report.Graph != testCase.wantTier {
				t.Fatalf("Graph = %+v, want %+v", *report.Graph, testCase.wantTier)
			}
		})
	}
}

func TestCheckStructureCountsCutFunctions(t *testing.T) {
	t.Parallel()

	runCutCases(t, []cutCase{
		{
			name:     "a graph of plain functions has no cut function",
			spec:     structureSpec{placed: map[string]int{"A": 0, "B": 1}},
			wantTier: GraphTier{},
		},
		{
			name: "a head with its statements is one cut function",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "F#1": 1,
			}},
			wantTier: GraphTier{CutFunctions: 1},
		},
		{
			name: "a type is never a cut function",
			spec: structureSpec{placed: map[string]int{
				"F": 0, "F#0": 0, "type:T": 2,
			}},
			wantTier: GraphTier{CutFunctions: 1},
		},
	})
}

func TestCheckStructureRefusesACutFunctionWithAPieceInNoRow(t *testing.T) {
	t.Parallel()

	runCutCases(t, []cutCase{
		{
			name: "a statement the manifest never births is refused and named",
			spec: structureSpec{
				placed:   map[string]int{"F": 0, "F#0": 0},
				unplaced: []string{"F#1"},
			},
			wantErr:   ErrMismatch,
			wantNamed: "a.go:func:F",
		},
		{
			name: "a head the manifest never births is refused and named",
			spec: structureSpec{
				placed:   map[string]int{"F#0": 0, "F#1": 1},
				unplaced: []string{"F"},
			},
			wantErr:   ErrMismatch,
			wantNamed: "a.go:func:F",
		},
		{
			name: "a function whose every piece is unplaced is refused and named",
			spec: structureSpec{
				placed:   map[string]int{"A": 0},
				unplaced: []string{"F", "F#0", "F#1"},
			},
			wantErr:   ErrMismatch,
			wantNamed: "a.go:func:F",
		},
	})
}

func TestCheckStructureReadsOnlyABodyEdgeIntoItsOwnFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		providerFile string
		reason       entities.EdgeReason
		wantCut      int
	}{
		{
			name:         "a body edge into the same file makes a cut function",
			providerFile: "a.go",
			reason:       entities.ReasonBody,
			wantCut:      1,
		},
		{
			name:         "a body edge into another file is no cut function",
			providerFile: "b.go",
			reason:       entities.ReasonBody,
		},
		{
			name:         "a reference edge into the same file is no cut function",
			providerFile: "a.go",
			reason:       entities.ReasonReference,
		},
		{
			name:         "an enclosing edge into the same file is no cut function",
			providerFile: "a.go",
			reason:       entities.ReasonEncloses,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			head := entities.DeclKey{
				File: testCase.providerFile, Kind: entities.KindFunc, Name: "G",
			}
			stmt := entities.DeclKey{File: "a.go", Kind: entities.KindStmt, Name: "G#0"}
			graph := entities.NewGraph(
				[]entities.Decl{{Key: head, Unit: "b"}, {Key: stmt, Unit: "a"}},
				[]entities.Edge{
					{Dependent: stmt, Provider: head, Reason: testCase.reason},
				}, nil,
			)

			report, err := CheckStructure(context.Background(), Structure{
				Graph: graph, Born: map[entities.DeclKey]int{head: 0, stmt: 1},
				Syntax: syntaxMock{},
			}, DefaultLimits())
			if err != nil || report.Graph.CutFunctions != testCase.wantCut {
				t.Fatalf(
					"CheckStructure() = %+v, %v, want %d cut function",
					report.Graph, err, testCase.wantCut,
				)
			}
		})
	}
}
