package dart

import "strings"

// skipSpaceAndComments reports whether input remains after skipping.
func (l *lexer) skipSpaceAndComments() bool {
	for l.pos < len(l.src) {
		switch {
		case isSpace(l.src[l.pos]):
			l.pos++
		case strings.HasPrefix(l.src[l.pos:], "//"):
			l.skipLine()
		case strings.HasPrefix(l.src[l.pos:], "/*"):
			l.skipBlock()
		default:
			return true
		}
	}

	return false
}

func (l *lexer) skipLine() {
	if end := strings.IndexByte(l.src[l.pos:], '\n'); end >= 0 {
		l.pos += end + 1

		return
	}

	l.pos = len(l.src)
}

// skipBlock skips a block comment; Dart nests them.
func (l *lexer) skipBlock() {
	depth := 0

	for l.pos < len(l.src) {
		switch {
		case strings.HasPrefix(l.src[l.pos:], "/*"):
			depth++
			l.pos += 2
		case strings.HasPrefix(l.src[l.pos:], "*/"):
			depth--
			l.pos += 2

			if depth == 0 {
				return
			}
		default:
			l.pos++
		}
	}
}
