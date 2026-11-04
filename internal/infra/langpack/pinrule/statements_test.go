package pinrule

import (
	"context"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestPackEdgesCutFunction(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		declOf("main.go", "main"),
		stmtOf("main.go", "main#0"),
		stmtOf("main.go", "main#1"),
		stmtOf("other.go", "main#0"),
		declOf("p.go", "Z"),
	}
	keyMain, keyFirstStmt, keyLastStmt, keyZ := decls[0].Key, decls[1].Key, decls[2].Key, decls[4].Key

	testCases := []struct {
		name      string
		config    string
		wantEdges []entities.Edge
	}{
		{
			name:   "a pin after a cut function waits for its signature and every statement",
			config: `{"type":"pin","decl":"p.go:func:Z","after":"main.go:func:main"}`,
			wantEdges: []entities.Edge{
				{Dependent: keyZ, Provider: keyMain, Reason: entities.ReasonPin},
				{Dependent: keyZ, Provider: keyFirstStmt, Reason: entities.ReasonPin},
				{Dependent: keyZ, Provider: keyLastStmt, Reason: entities.ReasonPin},
			},
		},
		{
			name:   "a statement of the dependent's own function adds no edge from itself",
			config: `{"type":"pin","decl":"main.go:*:main*","after":"main.go:func:main"}`,
			wantEdges: []entities.Edge{
				{Dependent: keyFirstStmt, Provider: keyMain, Reason: entities.ReasonPin},
				{Dependent: keyLastStmt, Provider: keyMain, Reason: entities.ReasonPin},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := packOf(t, []string{testCase.config}).
				Edges(context.Background(), entities.Snapshot{}, decls)
			if err != nil {
				t.Fatalf("Edges() = %v", err)
			}

			if !slices.Equal(got, testCase.wantEdges) {
				t.Fatalf("Edges() = %v, want %v", got, testCase.wantEdges)
			}
		})
	}
}
