package sqlstmt

import (
	"slices"
	"strings"
)

// describe names a statement from its leading keywords. A statement it does not
// recognise is Other with no name, which Split numbers.
func describe(tokens []token) Statement {
	switch {
	case startsWith(tokens, "CREATE", "TABLE"):
		table := identAfter(tokens, "TABLE", "IF", "NOT", "EXISTS")

		return Statement{
			Kind: CreateTable, Name: "table:" + table, Table: table, Refs: refsOf(tokens),
		}
	case startsWith(tokens, "CREATE", "INDEX"), startsWith(tokens, "CREATE", "UNIQUE", "INDEX"):
		return Statement{
			Kind:  CreateIndex,
			Name:  "index:" + identAfter(tokens, "INDEX", "CONCURRENTLY", "IF", "NOT", "EXISTS"),
			Table: identAfter(tokens, "ON", "ONLY"),
		}
	case startsWith(tokens, "ALTER", "TABLE"):
		table := identAfter(tokens, "TABLE", "IF", "EXISTS", "ONLY")

		return Statement{
			Kind:  AlterTable,
			Name:  "alter:" + table,
			Table: table,
			Refs:  refsOf(tokens),
		}
	case startsWith(tokens, "CREATE", "EXTENSION"):
		return Statement{
			Kind: Extension,
			Name: "extension:" + identAfter(tokens, "EXTENSION", "IF", "NOT", "EXISTS"),
		}
	}

	return Statement{Kind: Other}
}

func tokenText(tok token) string {
	if tok.kind == word {
		return strings.ToUpper(tok.text)
	}

	return ""
}

func startsWith(tokens []token, keywords ...string) bool {
	if len(tokens) < len(keywords) {
		return false
	}

	for index, keyword := range keywords {
		if tokenText(tokens[index]) != keyword {
			return false
		}
	}

	return true
}

// identAfter returns the name that follows the first occurrence of anchor,
// stepping over the optional keywords between them. A schema-qualified name
// reads as its last part.
func identAfter(tokens []token, anchor string, skip ...string) string {
	for index, tok := range tokens {
		if tokenText(tok) != anchor {
			continue
		}

		return nameFrom(tokens, index+1, skip)
	}

	return ""
}

func nameFrom(tokens []token, from int, skip []string) string {
	for from < len(tokens) && slices.Contains(skip, tokenText(tokens[from])) {
		from++
	}

	name := ""

	for from < len(tokens) {
		tok := tokens[from]
		if tok.kind != word && tok.kind != quoted {
			break
		}

		name = strings.ToLower(tok.text)
		from++

		if from >= len(tokens) || tokens[from].kind != punct || tokens[from].text != "." {
			break
		}

		from++
	}

	return name
}

// refsOf lists the tables a statement's REFERENCES clauses name, once each.
func refsOf(tokens []token) []string {
	var refs []string

	for index, tok := range tokens {
		if tokenText(tok) != "REFERENCES" {
			continue
		}

		name := nameFrom(tokens, index+1, nil)
		if name != "" && !slices.Contains(refs, name) {
			refs = append(refs, name)
		}
	}

	return refs
}
