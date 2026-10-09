package dart

import "strings"

type tokenKind uint8

const (
	tokIdent tokenKind = iota + 1
	tokString
	tokNumber
	tokPunct
)

// token is one lexeme. A string literal is a single token whose inner holds
// what its interpolations lex to: the splitter must not see their braces, and
// the reference scan must still see their identifiers.
type token struct {
	kind  tokenKind
	text  string
	value string
	start int
	end   int
	inner []token
}

// operators are the multi-character punctuators the splitter has to tell
// apart from a lone "=": an item ends at ";" only once it saw a top-level
// "=" or "=>", and "==" or "<=" must not count as one.
var operators = [...]string{
	"...", "??=", "~/=", "?..", "=>", "==", "!=", "<=", ">=", "..", "?.", "??", "&&", "||",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=",
}

type lexer struct {
	src string
	pos int
}

func lex(source string) []token {
	l := lexer{src: source}

	return l.run(false)
}

// run lexes until the end of input or, inside an interpolation, until the
// brace that closes it.
func (l *lexer) run(isInterpolation bool) []token {
	var (
		out   []token
		depth int
	)

	for l.skipSpaceAndComments() {
		ch := l.src[l.pos]

		switch {
		case ch == '}' && isInterpolation && depth == 0:
			l.pos++

			return out
		case isIdentStart(ch):
			out = append(out, l.identOrRaw())
		case ch == '\'' || ch == '"':
			out = append(out, l.str(l.pos, false))
		case isDigit(ch):
			out = append(out, l.number())
		default:
			tok := l.punct()
			depth += braceDelta(tok.text)
			out = append(out, tok)
		}
	}

	return out
}

func braceDelta(text string) int {
	switch text {
	case "{":
		return 1
	case "}":
		return -1
	default:
		return 0
	}
}

func (l *lexer) identOrRaw() token {
	start := l.pos

	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		l.pos++
	}

	if l.pos-start == 1 && l.src[start] == 'r' && l.pos < len(l.src) &&
		(l.src[l.pos] == '\'' || l.src[l.pos] == '"') {
		return l.str(start, true)
	}

	return token{kind: tokIdent, text: l.src[start:l.pos], start: start, end: l.pos}
}

func (l *lexer) number() token {
	start := l.pos

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		isFraction := ch == '.' && l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1])

		if !isIdentPart(ch) && !isFraction {
			break
		}

		l.pos++
	}

	return token{kind: tokNumber, text: l.src[start:l.pos], start: start, end: l.pos}
}

func (l *lexer) punct() token {
	start := l.pos

	for _, op := range operators {
		if strings.HasPrefix(l.src[l.pos:], op) {
			l.pos += len(op)

			return token{kind: tokPunct, text: op, start: start, end: l.pos}
		}
	}

	l.pos++

	return token{kind: tokPunct, text: l.src[start:l.pos], start: start, end: l.pos}
}

func isSpace(ch byte) bool { return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' }

func isDigit(ch byte) bool { return ch >= '0' && ch <= '9' }

func isIdentStart(ch byte) bool {
	return ch == '_' || ch == '$' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		ch >= 0x80
}

func isIdentPart(ch byte) bool { return isIdentStart(ch) || isDigit(ch) }
