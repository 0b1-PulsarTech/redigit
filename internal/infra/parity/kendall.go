package parity

import (
	"math"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type KendallResult struct {
	Measure entities.Measure
	Tau     float64
	Defined bool
	Common  int
}

const pairSize = 2

type rankPair struct{ left, right int }

func KendallTau(left, right []Row) KendallResult {
	leftFirst, rightFirst := firstPositions(left), firstPositions(right)

	pairs := make([]rankPair, 0, len(leftFirst))

	for key, leftPos := range leftFirst {
		if rightPos, found := rightFirst[key]; found {
			pairs = append(pairs, rankPair{left: leftPos, right: rightPos})
		}
	}

	slices.SortFunc(pairs, func(first, second rankPair) int {
		if first.left != second.left {
			return first.left - second.left
		}

		return first.right - second.right
	})

	size := len(pairs)
	total := size * (size - 1) / pairSize
	tiedLeft := tiedPairs(pairs, func(pair rankPair) [2]int { return [2]int{pair.left, 0} })
	tiedBoth := tiedPairs(pairs, func(pair rankPair) [2]int {
		return [2]int{pair.left, pair.right}
	})

	rights := make([]int, size)
	for idx, pair := range pairs {
		rights[idx] = pair.right
	}

	discordant := countInversions(slices.Clone(rights), make([]int, size))

	slices.Sort(rights)

	tiedRight := tiedRuns(rights)
	concordant := total - tiedLeft - tiedRight + tiedBoth - discordant

	tau, defined := 0.0, false
	if denom := math.Sqrt(
		float64(total-tiedLeft),
	) * math.Sqrt(
		float64(total-tiedRight),
	); denom > 0 {
		tau, defined = float64(concordant-discordant)/denom, true
	}

	return KendallResult{
		Measure: entities.Measure{
			Name:  "kendall-tau-concordant-pairs",
			Value: concordant,
			Total: total,
			Excluded: []string{
				"chunk keys present on one side only",
				"only the first position of a key counts, later copies are ignored",
				"pairs tied on either side are in Total but neither concordant nor discordant",
				"Tau is undefined (Defined false, Tau 0) with fewer than two common keys or a side that is all ties",
			},
		},
		Tau:     tau,
		Defined: defined,
		Common:  size,
	}
}

func tiedPairs(sorted []rankPair, project func(rankPair) [2]int) int {
	tied, run := 0, 0

	for idx := range sorted {
		if idx > 0 && project(sorted[idx]) == project(sorted[idx-1]) {
			run++
			tied += run
		} else {
			run = 0
		}
	}

	return tied
}

func tiedRuns(sorted []int) int {
	tied, run := 0, 0

	for idx := range sorted {
		if idx > 0 && sorted[idx] == sorted[idx-1] {
			run++
			tied += run
		} else {
			run = 0
		}
	}

	return tied
}

func countInversions(values, scratch []int) int {
	// Counts strictly descending pairs and leaves values sorted.
	if len(values) < pairSize {
		return 0
	}

	mid := len(values) / pairSize
	count := countInversions(
		values[:mid],
		scratch[:mid],
	) + countInversions(
		values[mid:],
		scratch[mid:],
	)

	lo, hi, out := 0, mid, 0
	for lo < mid && hi < len(values) {
		if values[hi] < values[lo] {
			count += mid - lo
			scratch[out] = values[hi]
			hi++
		} else {
			scratch[out] = values[lo]
			lo++
		}

		out++
	}

	out += copy(scratch[out:], values[lo:mid])
	copy(scratch[out:], values[hi:])
	copy(values, scratch)

	return count
}
