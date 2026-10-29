package parity

import (
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type RowSetResult struct {
	Measure entities.Measure
	Differs []int
}

func RowSetEquality(left, right []Row) (RowSetResult, error) {
	leftSets, err := keySets(left)
	if err != nil {
		return RowSetResult{}, fmt.Errorf("left plan: %w", err)
	}

	rightSets, err := keySets(right)
	if err != nil {
		return RowSetResult{}, fmt.Errorf("right plan: %w", err)
	}

	positions := make(map[int]struct{}, len(leftSets))
	for position := range leftSets {
		positions[position] = struct{}{}
	}

	for position := range rightSets {
		positions[position] = struct{}{}
	}

	differs := make([]int, 0)

	for position := range positions {
		leftKeys, inLeft := leftSets[position]

		rightKeys, inRight := rightSets[position]
		if !inLeft || !inRight || !slices.Equal(leftKeys, rightKeys) {
			differs = append(differs, position)
		}
	}

	slices.Sort(differs)

	return RowSetResult{
		Measure: entities.Measure{
			Name:  "row-set-equality",
			Value: len(positions) - len(differs),
			Total: len(positions),
			Excluded: []string{
				"order of chunks inside a commit",
				"chunk spans, units and blob contents",
				"subject, shape and feature of the commit",
				"a position present on one side only counts as different",
				"a repeated position within one plan is an error, not merged",
			},
		},
		Differs: differs,
	}, nil
}

func (result RowSetResult) Fraction() (fraction float64, defined bool) {
	if result.Measure.Total == 0 {
		return 0, false
	}

	return float64(result.Measure.Value) / float64(result.Measure.Total), true
}
