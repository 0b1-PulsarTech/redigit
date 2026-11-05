package tile

import (
	"cmp"
	"errors"
	"slices"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	ErrOffsetRange      = errors.New("offset is outside the source")
	ErrOffsetInsideRune = errors.New("offset falls inside a character")
)

func UTF16ToBytes(source []byte, offsets []int) ([]int, error) {
	out := make([]int, len(offsets))

	var (
		pos, units int
		bad        *IndexError
	)

	// Each invalid byte is one U+FFFD unit and a BOM is one unit, three bytes.
	for _, idx := range sortedOrder(offsets) {
		target := offsets[idx]
		if target < 0 {
			bad = lowestIndex(bad, idx, ErrOffsetRange)

			continue
		}

		for units < target && pos < len(source) {
			char, size := utf8.DecodeRune(source[pos:])

			width := utf16.RuneLen(char)
			if units+width > target {
				break
			}

			units += width
			pos += size
		}

		switch {
		case units == target:
			out[idx] = pos
		case pos == len(source):
			bad = lowestIndex(bad, idx, ErrOffsetRange)
		default:
			bad = lowestIndex(bad, idx, ErrOffsetInsideRune)
		}
	}

	if bad != nil {
		return nil, bad
	}

	return out, nil
}

func BytesToUTF16(source []byte, offsets []int) ([]int, error) {
	out := make([]int, len(offsets))

	var (
		pos, units int
		bad        *IndexError
	)

	// Each invalid byte is one U+FFFD unit and a BOM is one unit, three bytes.
	for _, idx := range sortedOrder(offsets) {
		target := offsets[idx]
		if target < 0 || target > len(source) {
			bad = lowestIndex(bad, idx, ErrOffsetRange)

			continue
		}

		for pos < target {
			char, size := utf8.DecodeRune(source[pos:])
			pos += size
			units += utf16.RuneLen(char)
		}

		if pos != target {
			bad = lowestIndex(bad, idx, ErrOffsetInsideRune)

			continue
		}

		out[idx] = units
	}

	if bad != nil {
		return nil, bad
	}

	return out, nil
}

func lowestIndex(current *IndexError, index int, err error) *IndexError {
	if current != nil && current.Index < index {
		return current
	}

	return &IndexError{Index: index, Err: err}
}

func sortedOrder(offsets []int) []int {
	order := make([]int, len(offsets))
	for idx := range order {
		order[idx] = idx
	}

	if slices.IsSorted(offsets) {
		return order
	}

	slices.SortFunc(order, func(left, right int) int {
		return cmp.Or(cmp.Compare(offsets[left], offsets[right]), cmp.Compare(left, right))
	})

	return order
}
