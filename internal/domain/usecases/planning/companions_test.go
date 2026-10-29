package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestShareCompanionsFollowsTheTypeAFunctionBuilds(t *testing.T) {
	t.Parallel()

	repo := decl("store/repo.go", "Repo", "store")
	repo.Key.Kind = entities.KindType
	build := decl("store/repo.go", "New", "store")
	elsewhere := decl("other/repo.go", "Build", "other")
	plain := decl("store/util.go", "Helper", "store")
	checkTest := testDecl("store/repo_test.go", "newFake", "store")
	graph := entities.NewGraph(
		[]entities.Decl{repo, build, elsewhere, plain, checkTest},
		[]entities.Edge{
			needs(build, repo), needs(elsewhere, repo), needs(checkTest, repo),
		},
		nil,
	)
	own := ownership{
		owner: map[entities.DeclKey]FeatureID{
			repo.Key: reach.SharedFeature, build.Key: "wire/di", elsewhere.Key: "wire/di",
			plain.Key: "wire/di", checkTest.Key: "wire/di",
		},
		deferred: reach.Wiring{},
	}

	shareCompanions(graph, own)

	testCases := []struct {
		name string
		decl entities.Decl
		want FeatureID
	}{
		{"a constructor of a shared type is shared with it", build, reach.SharedFeature},
		{"a function of another package keeps its feature", elsewhere, "wire/di"},
		{"a function that builds no type keeps its feature", plain, "wire/di"},
		{"a test-side declaration keeps its feature", checkTest, "wire/di"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := own.owner[testCase.decl.Key]; got != testCase.want {
				t.Fatalf("owner = %q, want %q", got, testCase.want)
			}
		})
	}
}
