package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestReadyDeferredBringsWhatNoBuiltFeatureReaches(t *testing.T) {
	t.Parallel()

	suite := testDecl("e2e/flow_test.go", "TestFlow", "e2e")
	require := decl("e2e/go.mod", "require x/y", "e2e")
	require.Key.Kind = entities.KindOther
	indirect := decl("e2e/go.mod", "require x/z // indirect", "e2e")
	indirect.Key.Kind = entities.KindOther
	query := decl("db/q.sql", "Insert", "db")
	query.Key.Kind = entities.KindOther
	feature := decl("a/handler.go", "Handle", "a")
	counted := decl("a/count.go", "Count", "a")

	graph := entities.NewGraph(
		[]entities.Decl{suite, require, indirect, query, feature, counted},
		[]entities.Edge{
			needs(suite, require), needs(suite, query), needs(suite, counted),
			{Dependent: suite.Key, Provider: require.Key, Reason: entities.ReasonImport},
			needs(feature, counted),
			{Dependent: require.Key, Provider: indirect.Key, Reason: entities.ReasonImport},
		},
		nil,
	)

	testCases := []struct {
		name  string
		reach map[entities.DeclKey]int
		owner map[entities.DeclKey]FeatureID
		want  bool
	}{
		{
			"a require nothing but the suite imports",
			map[entities.DeclKey]int{require.Key: 1, indirect.Key: 1, query.Key: 0, counted.Key: 1},
			map[entities.DeclKey]FeatureID{counted.Key: SharedFeature},
			true,
		},
		{
			"a query no built feature reaches",
			map[entities.DeclKey]int{require.Key: 0, query.Key: 0, counted.Key: 1},
			map[entities.DeclKey]FeatureID{counted.Key: SharedFeature},
			true,
		},
		{
			"shared code a feature also reads still waits for its feature",
			map[entities.DeclKey]int{require.Key: 0, query.Key: 2, counted.Key: 1},
			map[entities.DeclKey]FeatureID{counted.Key: SharedFeature},
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			owner := map[entities.DeclKey]FeatureID{
				suite.Key: SharedFeature, require.Key: SharedFeature, indirect.Key: SharedFeature,
				query.Key:   SharedFeature,
				counted.Key: SharedFeature, feature.Key: "a",
			}
			own := ownership{
				owner:    owner,
				reach:    testCase.reach,
				deferred: Wiring{suite.Key: true},
				roots:    Wiring{},
			}
			born := map[entities.DeclKey]bool{counted.Key: true}
			fresh := map[entities.DeclKey]bool{counted.Key: true}

			got := slices.Contains(readyDeferred(graph, own, born, fresh), suite.Key)
			if got != testCase.want {
				t.Fatalf("suite ready = %v, want %v", got, testCase.want)
			}
		})
	}
}
