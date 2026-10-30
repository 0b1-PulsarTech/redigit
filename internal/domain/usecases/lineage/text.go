package lineage

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (scanner Scanner) matchText(
	ctx context.Context,
	blobs [2]string,
	oldLeft, newLeft []entities.Decl,
) ([]declPair, error) {
	oldSrc, err := scanner.log.ReadBlob(ctx, blobs[0])
	if err == nil {
		var newSrc []byte

		if newSrc, err = scanner.log.ReadBlob(ctx, blobs[1]); err == nil {
			return rankPairs(candidates(shinglesAt(oldSrc, oldLeft), shinglesAt(newSrc, newLeft),
				oldLeft, newLeft)), nil
		}
	}

	if errors.Is(err, ErrBlobMissing) {
		return nil, nil
	}

	return nil, fmt.Errorf("scan lineage: read renamed text: %w", err)
}

func shinglesAt(src []byte, decls []entities.Decl) []shingles {
	out := make([]shingles, len(decls))

	for idx, decl := range decls {
		span := decl.Span
		if span.Start < 0 || span.End > len(src) || span.Start >= span.End {
			continue
		}

		out[idx] = shinglesOf(tokenize(src[span.Start:span.End]))
	}

	return out
}

func candidates(
	oldSets, newSets []shingles,
	oldLeft, newLeft []entities.Decl,
) []candidate {
	var out []candidate

	for newIdx, newSet := range newSets {
		for oldIdx, oldSet := range oldSets {
			score, isSimilar := similarity(oldSet, newSet)
			if !isSimilar {
				continue
			}

			out = append(out, candidate{
				declPair: declPair{
					from: oldLeft[oldIdx].Key, to: newLeft[newIdx].Key,
					textScore: score,
				},
				oldIdx: oldIdx, newIdx: newIdx,
			})
		}
	}

	return out
}

func compareKeys(left, right entities.DeclKey) int {
	return cmp.Or(
		cmp.Compare(left.File, right.File),
		cmp.Compare(left.Kind, right.Kind),
		cmp.Compare(left.Name, right.Name),
	)
}

func rankPairs(found []candidate) []declPair {
	slices.SortFunc(found, func(left, right candidate) int {
		return cmp.Or(
			cmp.Compare(right.textScore, left.textScore),
			compareKeys(left.to, right.to),
			compareKeys(left.from, right.from),
		)
	})

	var (
		out      []declPair
		oldTaken = map[int]bool{}
		newTaken = map[int]bool{}
	)

	for _, one := range found {
		if oldTaken[one.oldIdx] || newTaken[one.newIdx] {
			continue
		}

		oldTaken[one.oldIdx], newTaken[one.newIdx] = true, true

		out = append(out, one.declPair)
	}

	return out
}
