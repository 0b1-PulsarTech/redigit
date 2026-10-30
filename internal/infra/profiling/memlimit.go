package profiling

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var ErrMemLimit = errors.New("bad memory limit")

func ParseMemLimit(text string) (int64, error) {
	text = strings.TrimSpace(text)
	if text == "off" {
		return math.MaxInt64, nil
	}

	const (
		kibibyte = 1024
		mebibyte = 1024 * kibibyte
		gibibyte = 1024 * mebibyte
		tebibyte = 1024 * gibibyte
	)

	units := []struct {
		suffix string
		scale  int64
	}{
		{"TiB", tebibyte},
		{"GiB", gibibyte},
		{"MiB", mebibyte},
		{"KiB", kibibyte},
		{"B", 1},
	}

	scale := int64(1)

	for _, unit := range units {
		if digits, found := strings.CutSuffix(text, unit.suffix); found {
			text, scale = digits, unit.scale

			break
		}
	}

	count, err := strconv.ParseInt(text, 10, 64)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("%w: want an integer and an optional unit", ErrMemLimit)
	}

	// A zero limit makes the collector run flat out, which no one asks for.
	if count == 0 {
		return 0, fmt.Errorf("%w: the limit must be above zero", ErrMemLimit)
	}

	if count > math.MaxInt64/scale {
		return 0, fmt.Errorf("%w: the limit overflows 64 bits", ErrMemLimit)
	}

	return count * scale, nil
}
