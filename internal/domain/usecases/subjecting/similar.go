package subjecting

import (
	"maps"
	"slices"
)

func matchSimilar(result *Carried, sources []source, current []Entry, claimed map[int]bool) {
	index := make(map[string][]int)
	sizes := make([]int, len(current))

	for idx, entry := range current {
		items := itemsOf(entry)
		sizes[idx] = len(items)

		for _, item := range items {
			index[item] = append(index[item], idx)
		}
	}

	proposals := make(map[int][]Match)

	for _, src := range sources {
		target, score, isUnique := bestOf(itemsOf(src.entry), index, sizes, claimed)
		if !isUnique || score < SimilarityFloor {
			result.Lost = append(result.Lost, Lost{From: src.entry.Position, Subject: src.subject})

			continue
		}

		proposals[target] = append(proposals[target], Match{
			From: src.entry.Position, To: current[target].Position, ID: current[target].ID,
			Tier: TierReview, Score: score, Subject: src.subject,
		})
	}

	for _, target := range slices.Sorted(maps.Keys(proposals)) {
		if len(proposals[target]) > 1 {
			for _, match := range proposals[target] {
				result.Lost = append(result.Lost, Lost{From: match.From, Subject: match.Subject})
			}

			continue
		}

		result.Review = append(result.Review, proposals[target][0])
	}
}

func bestOf(
	items []string,
	index map[string][]int,
	sizes []int,
	claimed map[int]bool,
) (target int, score float64, isUnique bool) {
	shared := make(map[int]int)

	for _, item := range items {
		for _, idx := range index[item] {
			if !claimed[idx] {
				shared[idx]++
			}
		}
	}

	for _, idx := range slices.Sorted(maps.Keys(shared)) {
		jaccard := float64(shared[idx]) / float64(len(items)+sizes[idx]-shared[idx])

		switch {
		case jaccard > score:
			target, score, isUnique = idx, jaccard, true
		case jaccard == score:
			isUnique = false
		}
	}

	return target, score, isUnique
}

func itemsOf(entry Entry) []string {
	items := entry.Chunks
	if len(items) == 0 {
		items = entry.Blobs
	}

	return slices.Compact(slices.Sorted(slices.Values(items)))
}
