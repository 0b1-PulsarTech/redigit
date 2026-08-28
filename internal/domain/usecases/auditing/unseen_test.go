package auditing

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestResolverCouldSee(t *testing.T) {
	t.Parallel()

	open := decl("libs/kit/id.go", "NewID", "libs/kit")
	tagged := decl("libs/kit/id_integration_test.go", "TestNewID", "libs/kit")
	alsoTagged := decl("libs/kit/id_integration_test.go", "TestAgain", "libs/kit")

	graph := entities.NewGraph([]entities.Decl{open, tagged, alsoTagged}, nil, nil)

	testCases := []struct {
		name      string
		unseen    []Unseen
		wantValue int
	}{
		{
			name:      "nothing hidden counts every declaration",
			unseen:    nil,
			wantValue: 3,
		},
		{
			name: "a hidden file removes every declaration it holds",
			unseen: []Unseen{
				{File: "libs/kit/id_integration_test.go", Constraint: "//go:build integration"},
			},
			wantValue: 1,
		},
		{
			name: "a hidden file the graph does not hold changes nothing",
			unseen: []Unseen{
				{File: "libs/kit/absent.go", Constraint: "//go:build e2e"},
			},
			wantValue: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			measure := New(Config{Unseen: testCase.unseen}).resolverCouldSee(graph)

			if measure.Value != testCase.wantValue {
				t.Fatalf("value = %d, want %d", measure.Value, testCase.wantValue)
			}

			if measure.Total != 3 {
				t.Fatalf("total = %d, want 3", measure.Total)
			}

			// A measure with no blind spot never leaves this package, so the
			// measure this file adds has to carry its own.
			if err := measure.Validate(); err != nil {
				t.Fatalf("validate: %v", err)
			}
		})
	}
}

func TestAuditCarriesUnseenThrough(t *testing.T) {
	t.Parallel()

	hidden := []Unseen{
		{
			File:       "libs/kit/id_integration_test.go",
			Constraint: "//go:build integration",
			Tags:       []string{"integration"},
		},
	}

	graph, order, _ := wiring()

	result, err := New(Config{Unseen: hidden}).Audit(graph, order)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}

	if len(result.Unseen) != 1 {
		t.Fatalf("unseen = %d, want 1", len(result.Unseen))
	}

	if result.Unseen[0].File != hidden[0].File {
		t.Fatalf("unseen file = %q, want %q", result.Unseen[0].File, hidden[0].File)
	}
}
