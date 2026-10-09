package config

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// names reports whether text writes target as a whole path: not glued to the
// end of a longer one ("libs/foobar") nor to its start ("xlibs/foo"). A longer
// path below it names the target only when what follows is not itself a
// directory: "libs/foo/bar.go" names libs/foo, while "libs/foo/bar" names
// libs/foo/bar and leaves libs/foo to its own mention.
func names(text, target string, dirs map[string][]entities.DeclKey) bool {
	for from := 0; ; {
		at := strings.Index(text[from:], target)
		if at < 0 {
			return false
		}

		start, end := from+at, from+at+len(target)
		if startsPath(text, start) && !isNameChar(after(text, end)) &&
			!descends(text, end, target, dirs) {
			return true
		}

		from = start + 1
	}
}

// startsPath reports a path that begins at start: after a separator that is
// not part of a longer path, or after the "./" and "../" of a relative one.
func startsPath(text string, start int) bool {
	if start == 0 {
		return true
	}

	previous := text[start-1]
	if previous == '/' {
		return start >= 2 && text[start-2] == '.'
	}

	return !isNameChar(previous)
}

// descends reports a "/" at end followed by a segment that makes target plus
// that segment a directory of the code.
func descends(text string, end int, target string, dirs map[string][]entities.DeclKey) bool {
	if after(text, end) != '/' {
		return false
	}

	last := end + 1
	for last < len(text) && isNameChar(text[last]) {
		last++
	}

	_, isDir := dirs[target+text[end:last]]

	return isDir
}

func after(text string, at int) byte {
	if at >= len(text) {
		return ' '
	}

	return text[at]
}

func isNameChar(char byte) bool {
	return char == '_' || char == '-' ||
		char >= '0' && char <= '9' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}
