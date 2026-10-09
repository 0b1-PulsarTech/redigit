package dart

import "strings"

// str lexes a string literal opening at l.pos; start is where its token begins,
// which is one byte earlier for a raw literal's "r". value is the literal text
// only when no interpolation splits it, which is all a directive URI needs.
func (l *lexer) str(start int, isRaw bool) token {
	quote := l.src[l.pos : l.pos+1]
	if strings.HasPrefix(l.src[l.pos:], quote+quote+quote) {
		quote += quote + quote
	}

	l.pos += len(quote)
	bodyStart := l.pos

	var (
		inner   []token
		isPlain = true
	)

	for l.pos < len(l.src) && !strings.HasPrefix(l.src[l.pos:], quote) {
		ch := l.src[l.pos]

		switch {
		case ch == '\\' && !isRaw:
			l.pos += 2
		case ch == '$' && !isRaw:
			isPlain = false

			inner = append(inner, l.interpolation()...)
		case ch == '\n' && len(quote) == 1:
			return l.closeString(start, bodyStart, l.pos, inner, isPlain)
		default:
			l.pos++
		}
	}

	bodyEnd := min(l.pos, len(l.src))
	l.pos = min(l.pos+len(quote), len(l.src))

	return l.closeString(start, bodyStart, bodyEnd, inner, isPlain)
}

func (l *lexer) closeString(start, bodyStart, bodyEnd int, inner []token, isPlain bool) token {
	tok := token{kind: tokString, text: l.src[start:l.pos], start: start, end: l.pos, inner: inner}
	if isPlain {
		tok.value = l.src[bodyStart:bodyEnd]
	}

	return tok
}

// interpolation lexes "$name" or "${expr}" starting at the dollar sign.
func (l *lexer) interpolation() []token {
	l.pos++

	if l.pos < len(l.src) && l.src[l.pos] == '{' {
		l.pos++

		return l.run(true)
	}

	start := l.pos

	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) && l.src[l.pos] != '$' {
		l.pos++
	}

	if l.pos == start {
		return nil
	}

	return []token{{kind: tokIdent, text: l.src[start:l.pos], start: start, end: l.pos}}
}
