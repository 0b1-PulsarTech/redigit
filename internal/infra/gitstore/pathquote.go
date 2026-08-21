package gitstore

import "fmt"

// quoteOverhead is the two double-quote bytes a quoted path adds around its
// (possibly escaped) content.
const quoteOverhead = 2

// quotePath renders path the way fast-import needs it on an M/D command
// line: unquoted when plain, or C-style quoted when it holds a space, a
// quote, a backslash, or a control byte. An unquoted space was measured to
// make git fast-import reject the whole line as an unsupported command.
func quotePath(path string) string {
	if !needsQuote(path) {
		return path
	}

	quoted := make([]byte, 0, len(path)+quoteOverhead)
	quoted = append(quoted, '"')

	for i := range len(path) {
		quoted = appendQuotedByte(quoted, path[i])
	}

	quoted = append(quoted, '"')

	return string(quoted)
}

func appendQuotedByte(quoted []byte, b byte) []byte {
	switch {
	case b == '"' || b == '\\':
		return append(quoted, '\\', b)
	case b < 0x20 || b == 0x7f:
		return fmt.Appendf(quoted, `\%03o`, b)
	default:
		return append(quoted, b)
	}
}

func needsQuote(path string) bool {
	for i := range len(path) {
		b := path[i]
		if b == ' ' || b == '"' || b == '\\' || b < 0x20 || b == 0x7f {
			return true
		}
	}

	return false
}
