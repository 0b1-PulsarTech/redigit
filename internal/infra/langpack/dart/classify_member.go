package dart

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// memberShape reads a top-level function, getter, setter, or variable list. A
// "(" names a function only when an identifier precedes it: a record type
// "(int, int) pair" and a "Function(int)" type both open one first.
const (
	kwGet = "get"
	kwSet = "set"
)

func memberShape(toks []token, at int) shape {
	for at < len(toks) && slices.Contains(valueModifiers, toks[at].text) {
		at++
	}

	for index := at; index < len(toks); index++ {
		switch text := toks[index].text; text {
		case kwGet, kwSet:
			if found, isAccessor := accessorShape(toks, index); isAccessor {
				return found
			}
		case "(":
			before := identBefore(toks, index)
			if index == at || before < at || toks[before].kind != tokIdent ||
				toks[before].text == "Function" {
				index = closing(toks, index)

				continue
			}

			return named(toks, before, entities.KindFunc)
		case "=", ";", "=>", "{":
			return valueShape(toks, at)
		}
	}

	return valueShape(toks, at)
}

func accessorShape(toks []token, at int) (shape, bool) {
	if at+2 >= len(toks) || toks[at+1].kind != tokIdent {
		return shape{}, false
	}

	if !slices.Contains([]string{"=>", "{", "(", ";"}, toks[at+2].text) {
		return shape{}, false
	}

	found := named(toks, at+1, entities.KindFunc)
	if toks[at].text == kwSet {
		found.name += "="
	}

	return found, true
}

// valueShape collects every name a variable list binds. Before the first "="
// a "," may sit inside type arguments, so angle brackets are counted; after
// it a name is an identifier a "," introduces and "=", "," or ";" follows.
func valueShape(toks []token, at int) shape {
	var (
		names  []string
		first  = -1
		angle  int
		paren  int
		isInit bool
	)

	for index := at; index < len(toks); index++ {
		parenStep, angleStep := depthStep(toks[index].text)
		paren, angle = paren+parenStep, angle+angleStep

		if paren != 0 || index == at {
			continue
		}

		if name := boundName(toks, index, isInit, angle); name >= 0 && toks[name].kind == tokIdent {
			names = append(names, toks[name].text)

			if first < 0 {
				first = name
			}
		}

		isInit = isInit || toks[index].text == "="
	}

	if first < 0 {
		return shape{kind: itemDecl, declKind: entities.KindOther, nameAt: -1}
	}

	return shape{
		kind:     itemDecl,
		declKind: entities.KindValue,
		name:     names[0],
		names:    names,
		nameAt:   first,
	}
}

func depthStep(text string) (paren, angle int) {
	switch text {
	case "(", "[", "{":
		return 1, 0
	case ")", "]", "}":
		return -1, 0
	case "<":
		return 0, 1
	case ">":
		return 0, -1
	default:
		return 0, 0
	}
}

// boundName is the index of the name the token at index ends or introduces,
// or -1.
func boundName(toks []token, index int, isInit bool, angle int) int {
	ends := []string{"=", ",", ";"}

	switch {
	case !isInit && angle == 0 && slices.Contains(ends, toks[index].text):
		return index - 1
	case isInit && toks[index].text == "," && index+2 < len(toks) &&
		slices.Contains(ends, toks[index+2].text):
		return index + 1
	default:
		return -1
	}
}

// extensionMembers names what an extension adds: a member is reached as
// "value.member", where nothing names the extension itself, so a reader of the
// member would otherwise carry no edge to it.
func extensionMembers(toks []token) []string {
	open := slices.IndexFunc(toks, func(tok token) bool { return tok.text == "{" })
	if open < 0 {
		return nil
	}

	var (
		members []string
		depth   int
	)

	for index := open + 1; index < len(toks)-1; index++ {
		switch toks[index].text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		}

		if depth != 0 || toks[index].kind != tokIdent {
			continue
		}

		isCall := toks[index+1].text == "("
		isAccessor := slices.Contains([]string{kwGet, kwSet, "operator"}, toks[index-1].text)

		if (isCall || isAccessor) && !slices.Contains(members, toks[index].text) {
			members = append(members, toks[index].text)
		}
	}

	return members
}
