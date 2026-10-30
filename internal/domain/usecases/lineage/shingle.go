package lineage

import (
	"slices"
	"unicode/utf8"
)

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

type shingles []uint64

func isWordByte(char byte) bool {
	return char == '_' || char >= utf8.RuneSelf || (char >= '0' && char <= '9') ||
		(char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

func tokenize(text []byte) []uint64 {
	// A rough average that only sizes the first allocation.
	const bytesPerToken = 4

	tokens := make([]uint64, 0, len(text)/bytesPerToken)

	for pos := 0; pos < len(text); {
		char := text[pos]

		switch {
		case char == ' ' || char == '\t' || char == '\n' || char == '\r':
			pos++
		case isWordByte(char):
			hash := uint64(fnvOffset)

			for pos < len(text) && isWordByte(text[pos]) {
				hash ^= uint64(text[pos])
				hash *= fnvPrime
				pos++
			}

			tokens = append(tokens, hash)
		default:
			hash := (uint64(fnvOffset) ^ uint64(char)) * fnvPrime
			tokens = append(tokens, hash)
			pos++
		}
	}

	return tokens
}

func shinglesOf(tokens []uint64) shingles {
	const shingleSize = 3

	if len(tokens) == 0 {
		return nil
	}

	width := min(shingleSize, len(tokens))
	out := make(shingles, 0, len(tokens)-width+1)

	for start := 0; start+width <= len(tokens); start++ {
		hash := uint64(fnvOffset)
		for _, token := range tokens[start : start+width] {
			hash ^= token
			hash *= fnvPrime
		}

		out = append(out, hash)
	}

	slices.Sort(out)

	return slices.Compact(out)
}

func overlap(left, right shingles) (shared, union int) {
	for lo, hi := 0, 0; lo < len(left) && hi < len(right); {
		switch {
		case left[lo] == right[hi]:
			shared++
			lo++
			hi++
		case left[lo] < right[hi]:
			lo++
		default:
			hi++
		}
	}

	return shared, len(left) + len(right) - shared
}

func similarity(left, right shingles) (score int, isSimilar bool) {
	// 60 percent of the union is the same declaration under a new name.
	const floor = 60

	shared, union := overlap(left, right)
	if union == 0 {
		return 0, false
	}

	return shared * 100 / union, shared*100 >= floor*union
}
