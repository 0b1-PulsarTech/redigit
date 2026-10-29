package parity

import (
	"fmt"
	"slices"
)

func firstPositions(rows []Row) map[string]int {
	first := make(map[string]int)

	for _, row := range rows {
		for _, chunk := range row.Chunks {
			if known, found := first[chunk.Key]; !found || row.Position < known {
				first[chunk.Key] = row.Position
			}
		}
	}

	return first
}

func keySets(rows []Row) (map[int][]string, error) {
	sets := make(map[int][]string, len(rows))

	for _, row := range rows {
		// The emit format has one row per position, so a repeat is a corrupt input.
		if _, seen := sets[row.Position]; seen {
			return nil, fmt.Errorf("%w: duplicate position %d", ErrMalformed, row.Position)
		}

		keys := make([]string, 0, len(row.Chunks))
		for _, chunk := range row.Chunks {
			keys = append(keys, chunk.Key)
		}

		slices.Sort(keys)
		sets[row.Position] = slices.Compact(keys)
	}

	return sets, nil
}
