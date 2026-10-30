package batch

import (
	"strings"
	"unicode/utf8"
)

const stderrLimit = 8 << 10

type TailBuffer struct{ kept []byte }

func (tail *TailBuffer) Write(chunk []byte) (int, error) {
	tail.kept = append(tail.kept, chunk...)

	if over := len(tail.kept) - stderrLimit; over > 0 {
		for over < len(tail.kept) && !utf8.RuneStart(tail.kept[over]) {
			over++
		}

		tail.kept = append(tail.kept[:0], tail.kept[over:]...)
	}

	return len(chunk), nil
}

func (tail *TailBuffer) String() string { return strings.TrimSpace(string(tail.kept)) }
