package rewriting

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

var ErrManifest = errors.New("invalid manifest")

type (
	BlobRef struct {
		Path   string
		Mode   string
		Handle string
	}

	Row struct {
		Position int
		Subject  string
		Blobs    []BlobRef
	}
)

func OrderRows(rows []Row) ([]Row, error) {
	ordered := slices.Clone(rows)
	slices.SortStableFunc(ordered, func(left, right Row) int {
		return cmp.Compare(left.Position, right.Position)
	})

	for idx := 1; idx < len(ordered); idx++ {
		if ordered[idx].Position == ordered[idx-1].Position {
			return nil, fmt.Errorf("%w: a position appears twice", ErrManifest)
		}
	}

	return ordered, nil
}
