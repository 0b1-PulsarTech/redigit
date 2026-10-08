package sqlstmt

import (
	"strconv"
	"strings"
)

// Kind is what a statement does to the schema.
type Kind string

const (
	CreateTable Kind = "create-table"
	CreateIndex Kind = "create-index"
	AlterTable  Kind = "alter-table"
	Extension   Kind = "extension"
	Other       Kind = "other"
)

// Statement is one terminated statement and the lines of comment above it.
// The statements of a file tile it: End of one is Start of the next, so
// concatenating any subset in order writes a file that still parses.
type Statement struct {
	Start, End int
	Kind       Kind
	// Name is unique within its file and says what the statement is, e.g.
	// "table:users" or "index:user_email": a declaration name.
	Name string
	// Table is the table a table-level statement is about; empty for an
	// extension.
	Table string
	// Refs are the tables a statement's REFERENCES clauses name.
	Refs []string
}

// Split cuts the dump a schema tool writes - plain statements, one terminator
// each - into statements. It returns no statement for a file with none, so
// the caller can fall back to treating the file as one piece.
func Split(source []byte) []Statement {
	raws := statementsOf(source)
	out := make([]Statement, 0, len(raws))
	counter := map[string]int{}
	start := 0

	for _, raw := range raws {
		stmt := describe(raw.tokens)
		stmt.Start, stmt.End = start, raw.end
		stmt.Name = unique(counter, stmt.Name, len(out)+1)

		out = append(out, stmt)
		start = raw.end
	}

	if len(out) > 0 {
		out[len(out)-1].End = len(source)
	}

	return out
}

// Idents returns every identifier and word text mentions, lower-cased, in
// order: comments and string literals are skipped, a quoted identifier is
// unquoted. A caller asking which tables a query reads intersects it with the
// table names it knows.
func Idents(text []byte) []string {
	var words []string

	for _, token := range tokenize(text) {
		if token.kind == word || token.kind == quoted {
			words = append(words, strings.ToLower(token.text))
		}
	}

	return words
}

func unique(counter map[string]int, name string, ordinal int) string {
	if name == "" {
		name = "stmt:" + strconv.Itoa(ordinal)
	}

	counter[name]++
	if counter[name] == 1 {
		return name
	}

	return name + "#" + strconv.Itoa(counter[name])
}

type rawStatement struct {
	tokens []token
	end    int
}

// statementsOf groups the tokens of source by the semicolons that end them. A
// statement's end takes the rest of its line, so the next one starts on a
// fresh line and the comment above it travels with it.
func statementsOf(source []byte) []rawStatement {
	var (
		out     []rawStatement
		pending []token
	)

	for _, tok := range tokenize(source) {
		if tok.kind != semicolon {
			pending = append(pending, tok)

			continue
		}

		out = append(out, rawStatement{tokens: pending, end: endOfLine(source, tok.end)})
		pending = nil
	}

	return out
}

func endOfLine(source []byte, from int) int {
	for at := from; at < len(source); at++ {
		switch source[at] {
		case '\n':
			return at + 1
		case ' ', '\t', '\r':
		default:
			return from
		}
	}

	return len(source)
}
