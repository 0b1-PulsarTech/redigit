package tile

import "errors"

var (
	ErrSpanInverted = errors.New("span start is after its end")
	ErrSpanBounds   = errors.New("span is outside the source")
	ErrSpanOrder    = errors.New("span overlaps or precedes the previous span")
)

type IndexError struct {
	Index int
	Err   error
}

var _ error = (*IndexError)(nil)

func (indexErr *IndexError) Error() string {
	if indexErr.Err == nil {
		return "bad index"
	}

	return indexErr.Err.Error()
}

func (indexErr *IndexError) Unwrap() error {
	return indexErr.Err
}

func Validate(source []byte, spans []Span) error {
	prevEnd := 0

	for index, span := range spans {
		switch {
		case span.Start > span.End:
			return &IndexError{Index: index, Err: ErrSpanInverted}
		case span.Start < 0 || span.End > len(source):
			return &IndexError{Index: index, Err: ErrSpanBounds}
		case span.Start < prevEnd:
			return &IndexError{Index: index, Err: ErrSpanOrder}
		}

		prevEnd = span.End
	}

	return nil
}
