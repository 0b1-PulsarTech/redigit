package sql

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestDropSqlcConstruction(t *testing.T) {
	t.Parallel()

	snap, _ := schemaTree(t)

	newFunc := testkit.DeclKey("dbgen/db.go", entities.KindFunc, "New")
	method := testkit.DeclKey("dbgen/account.sql.go", entities.KindMethod, "Queries.GetAccount")
	caller := testkit.DeclKey("repo/repo.go", entities.KindMethod, "Repo.Get")

	testCases := []struct {
		name string
		edge entities.Edge
		kept bool
	}{
		{
			"the constructor waiting for a query method",
			edge(newFunc, method, entities.ReasonImplements),
			false,
		},
		{"the same through a method set", edge(newFunc, method, entities.ReasonMethodSet), false},
		{
			"a plain reference inside the output",
			edge(method, newFunc, entities.ReasonReference),
			true,
		},
		{
			"a repository waiting for a query method",
			edge(caller, method, entities.ReasonImplements),
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := DropConstruction(snap, []entities.Edge{testCase.edge})
			if kept := slices.Contains(got, testCase.edge); kept != testCase.kept {
				t.Fatalf("kept = %v, want %v", kept, testCase.kept)
			}
		})
	}
}

func edge(dependent, provider entities.DeclKey, reason entities.EdgeReason) entities.Edge {
	return entities.Edge{Dependent: dependent, Provider: provider, Reason: reason}
}
