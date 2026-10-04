package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestPlaceStrandedTests(t *testing.T) {
	t.Parallel()

	shared := decl("kernel/k.go", "Kernel", "kernel")
	subjectA := decl("a/use.go", "UseA", "a")
	subjectB := decl("b/use.go", "UseB", "b")

	testCases := []struct {
		name  string
		needs []entities.Decl
		want  FeatureID
	}{
		{name: "one feature subject", needs: []entities.Decl{subjectA, shared}, want: "a"},
		{
			name:  "two feature subjects",
			needs: []entities.Decl{subjectA, subjectB},
			want:  LateFeature,
		},
		{name: "shared subject only", needs: []entities.Decl{shared}, want: SharedFeature},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			test := decl("x/x_test.go", "TestX", "x")
			test.IsTest = true

			edges := make([]entities.Edge, 0, len(testCase.needs))
			for _, provider := range testCase.needs {
				edges = append(edges, needs(test, provider))
			}

			graph := entities.NewGraph(
				[]entities.Decl{test, shared, subjectA, subjectB},
				edges,
				nil,
			)

			own := ownership{
				owner: map[entities.DeclKey]FeatureID{
					test.Key: SharedFeature, shared.Key: SharedFeature,
					subjectA.Key: "a", subjectB.Key: "b",
				},
				reach: map[entities.DeclKey]int{},
			}

			placeStrandedTests(graph, own)

			if got := own.owner[test.Key]; got != testCase.want {
				t.Fatalf("test owner = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestPlaceStrandedTestsFollowsAHelperToTheTrailingLayer(t *testing.T) {
	t.Parallel()

	subjectA := decl("a/use.go", "UseA", "a")
	subjectB := decl("b/use.go", "UseB", "b")
	support := decl("x/helper_test.go", "support", "x")
	support.IsTest = true
	test := decl("x/x_test.go", "TestX", "x")
	test.IsTest = true

	graph := entities.NewGraph(
		[]entities.Decl{subjectA, subjectB, support, test},
		[]entities.Edge{needs(support, subjectA), needs(support, subjectB), needs(test, support)},
		nil,
	)

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{
			subjectA.Key: "a",
			subjectB.Key: "b",
			support.Key:  SharedFeature,
			test.Key:     SharedFeature,
		},
		reach: map[entities.DeclKey]int{},
	}

	placeStrandedTests(graph, own)

	for _, key := range []entities.DeclKey{support.Key, test.Key} {
		if got := own.owner[key]; got != LateFeature {
			t.Errorf("%s owner = %q, want the trailing layer", key, got)
		}
	}
}

func TestAssignKeepsAUseCaseOutOfTheSharedLayerForItsTest(t *testing.T) {
	t.Parallel()

	// An integration test no entry point reaches is shared by default, and what
	// it calls used to follow it there.
	entry := decl("a/main.go", "Main", "a")
	useCase := decl("a/usecase.go", "Escalate", "a")
	test := decl("a/escalate_integration_test.go", "TestEscalate", "a")
	test.IsTest = true

	own := assign(entities.NewGraph(
		[]entities.Decl{entry, useCase, test},
		[]entities.Edge{needs(entry, useCase), needs(test, useCase)},
		nil,
	))

	if got := own.owner[useCase.Key]; got != "a" {
		t.Fatalf("use case owner = %q, want its feature", got)
	}

	if got := own.owner[test.Key]; got != "a" {
		t.Fatalf("test owner = %q, want the feature it exercises", got)
	}
}
