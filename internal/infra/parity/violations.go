package parity

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const maxViolationExamples = 10

type Violation struct {
	Edge         Edge
	Reasons      []entities.EdgeReason
	DependentPos int
	ProviderPos  int
}

type ViolationResult struct {
	Measure  entities.Measure
	Examples []Violation
}

type edgePair struct{ dependent, provider string }

func Violations(plan []Row, hardEdges []Edge) ViolationResult {
	first := firstPositions(plan)

	// The reference keeps providers as a set, so a pair counts once whatever its reasons.
	reasons := make(map[edgePair][]entities.EdgeReason, len(hardEdges))
	order := make([]Edge, 0, len(hardEdges))

	for _, edge := range hardEdges {
		pair := edgePair{dependent: edge.Dependent, provider: edge.Provider}
		if _, seen := reasons[pair]; !seen {
			order = append(order, edge)
		}

		if !slices.Contains(reasons[pair], edge.Reason) {
			reasons[pair] = append(reasons[pair], edge.Reason)
		}
	}

	checked, count := 0, 0
	examples := make([]Violation, 0, maxViolationExamples)

	for _, edge := range order {
		dependentPos, hasDependent := first[edge.Dependent]

		providerPos, hasProvider := first[edge.Provider]
		if !hasDependent || !hasProvider {
			continue
		}

		checked++

		if providerPos <= dependentPos {
			continue
		}

		count++

		if len(examples) < maxViolationExamples {
			pair := edgePair{dependent: edge.Dependent, provider: edge.Provider}
			examples = append(examples, Violation{
				Edge:         edge,
				Reasons:      reasons[pair],
				DependentPos: dependentPos,
				ProviderPos:  providerPos,
			})
		}
	}

	return ViolationResult{
		Measure: entities.Measure{
			Name:  "hard-edge-violations",
			Value: count,
			Total: checked,
			Excluded: []string{
				"edges with an endpoint absent from the plan",
				"soft edges: prose, test-of, describes, generated-by, and implements" +
					" unless a non-mock function, statement or method reaches a method",
				"a provider born in the same commit as its dependent is not a violation",
				"only the first position of a chunk key is its birth",
				"Total and Value count distinct (dependent, provider) pairs, not edge lines",
				"only the edges in the fixture file: engine.py derived edges (" +
					"companions, workspace, dataflow, textual, sql-function, assert, method-set closure) are not recomputed",
			},
		},
		Examples: examples,
	}
}
