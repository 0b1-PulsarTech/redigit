package batch

import "strings"

const stderrLimit = 8 << 10

type tailBuffer struct{ kept []byte }

func (tail *tailBuffer) Write(chunk []byte) (int, error) {
	tail.kept = append(tail.kept, chunk...)
	if over := len(tail.kept) - stderrLimit; over > 0 {
		tail.kept = append(tail.kept[:0], tail.kept[over:]...)
	}

	return len(chunk), nil
}

func (tail *tailBuffer) String() string { return strings.TrimSpace(string(tail.kept)) }
