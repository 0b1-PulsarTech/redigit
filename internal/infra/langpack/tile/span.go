package tile

import "bytes"

type Span struct {
	Start     int
	End       int
	IsContent bool
}

func Tile(source []byte, spans []Span, keep func(index int) bool) []byte {
	if len(spans) == 0 {
		return nil
	}

	var (
		prevEnd    = spans[0].Start
		out        = make([]byte, 0, len(source))
		hasDropped bool
		hasContent bool
	)

	out = append(out, source[:prevEnd]...)

	for index, span := range spans {
		gap := source[prevEnd:span.Start]
		prevEnd = span.End

		if !keep(index) {
			hasDropped = true

			continue
		}

		// Trimming needs a dropped span before it, so keep-all stays exact
		// and a kept zero-width span never changes what a later gap does.
		if len(out) == 0 && hasDropped {
			gap = bytes.TrimLeft(gap, "\r\n")
		}

		out = append(out, gap...)
		out = append(out, source[span.Start:span.End]...)
		hasContent = hasContent || span.IsContent
	}

	if !hasContent {
		return nil
	}

	return append(out, source[prevEnd:]...)
}
