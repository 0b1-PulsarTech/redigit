package sqlstmt

import "unicode"

type tokenKind int

const (
	word tokenKind = iota
	quoted
	str
	punct
	semicolon
)

type token struct {
	kind tokenKind
	// text is the word, or the quoted identifier without its quotes.
	text string
	end  int
}

// tokenize reads source as SQL lexemes. A line comment is dropped, a string
// literal keeps only its kind, and a doubled quote inside either quoted form
// is the escape for itself.
func tokenize(source []byte) []token {
	var (
		out []token
		at  int
	)

	for at < len(source) {
		char := rune(source[at])

		switch {
		case char == '-' && at+1 < len(source) && source[at+1] == '-':
			at = skipLine(source, at)
		case char == '"' || char == '`':
			text, next := readQuoted(source, at, byte(char))
			out = append(out, token{kind: quoted, text: text, end: next})
			at = next
		case char == '\'':
			_, next := readQuoted(source, at, '\'')
			out = append(out, token{kind: str, end: next})
			at = next
		case char == ';':
			at++
			out = append(out, token{kind: semicolon, end: at})
		case isWordRune(char):
			next := at
			for next < len(source) && isWordRune(rune(source[next])) {
				next++
			}

			out = append(out, token{kind: word, text: string(source[at:next]), end: next})
			at = next
		default:
			at++

			if !unicode.IsSpace(char) {
				out = append(out, token{kind: punct, text: string(char), end: at})
			}
		}
	}

	return out
}

func isWordRune(char rune) bool {
	return char == '_' || char == '$' || unicode.IsLetter(char) || unicode.IsDigit(char)
}

func skipLine(source []byte, from int) int {
	for from < len(source) && source[from] != '\n' {
		from++
	}

	return from
}

// readQuoted reads the literal opening at source[at] and returns its text and
// the offset after the closing quote.
func readQuoted(source []byte, at int, quote byte) (string, int) {
	var text []byte

	for next := at + 1; next < len(source); next++ {
		if source[next] != quote {
			text = append(text, source[next])

			continue
		}

		if next+1 < len(source) && source[next+1] == quote {
			text = append(text, quote)
			next++

			continue
		}

		return string(text), next + 1
	}

	return string(text), len(source)
}
